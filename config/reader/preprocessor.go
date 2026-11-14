package reader

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
)

var envVariable = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ReplaceEnvVars expands placeholders in JSON string values. Object keys and
// number representations are preserved; environment data is never parsed as JSON.
func ReplaceEnvVars(raw []byte) ([]byte, error) {
	if !envVariable.Match(raw) {
		return raw, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("reader: expected one json value")
	}
	return json.Marshal(expandEnvStrings(value))
}

func expandEnvStrings(value any) any {
	switch value := value.(type) {
	case string:
		return envVariable.ReplaceAllStringFunc(value, replaceEnvVars)
	case []any:
		for i, item := range value {
			value[i] = expandEnvStrings(item)
		}
		return value
	case map[string]any:
		for key, item := range value {
			value[key] = expandEnvStrings(item)
		}
		return value
	default:
		return value
	}
}

func replaceEnvVars(element string) string {
	return os.Getenv(element[2 : len(element)-1])
}
