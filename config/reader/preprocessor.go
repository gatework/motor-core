package reader

import (
	"os"
	"regexp"
)

var envVariable = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

func ReplaceEnvVars(raw []byte) ([]byte, error) {
	if envVariable.Match(raw) {
		dataS := string(raw)
		res := envVariable.ReplaceAllStringFunc(dataS, replaceEnvVars)
		return []byte(res), nil
	}
	return raw, nil
}

func replaceEnvVars(element string) string {
	v := element[2 : len(element)-1]
	el := os.Getenv(v)
	return el
}
