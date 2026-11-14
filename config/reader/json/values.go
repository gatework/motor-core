package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	simple "github.com/bitly/go-simplejson"

	"github.com/gatework/motor-core/config/reader"
	"github.com/gatework/motor-core/config/source"
)

type jsonValues struct {
	sj *simple.Json
}

type jsonValue struct {
	*simple.Json
}

func newValues(ch *source.ChangeSet) (reader.Values, error) {
	sj := simple.New()
	data, err := reader.ReplaceEnvVars(ch.Data)
	if err != nil {
		return nil, err
	}
	if err := sj.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return &jsonValues{sj: sj}, nil
}

func (j *jsonValues) Get(path ...string) reader.Value {
	v := simple.New()
	v.SetPath(nil, cloneValue(j.sj.GetPath(path...).Interface()))
	return &jsonValue{Json: v}
}

func (j *jsonValues) Del(path ...string) {
	// delete the tree?
	if len(path) == 0 {
		j.sj = simple.New()
		return
	}

	if len(path) == 1 {
		j.sj.Del(path[0])
		return
	}

	vals := j.sj.GetPath(path[:len(path)-1]...)
	vals.Del(path[len(path)-1])
	j.sj.SetPath(path[:len(path)-1], vals.Interface())
}

func (j *jsonValues) Set(val any, path ...string) error {
	// Config.Set supplies a normalized tree so no user marshaler runs while its
	// lock is held. Direct Values users may still supply any JSON-encodable type.
	owned, ok := cloneJSONValue(val, make(map[jsonVisit]bool))
	if !ok {
		data, err := json.Marshal(val)
		if err != nil {
			return err
		}
		value, err := simple.NewJson(data)
		if err != nil {
			return err
		}
		owned = value.Interface()
	}
	j.sj.SetPath(path, owned)
	return nil
}

type jsonVisit struct {
	pointer uintptr
	kind    reflect.Kind
	length  int
}

// Recognize only plain JSON trees. Cyclic or custom input goes through the JSON
// encoder, which reports invalid values and preserves custom marshaling rules.
func cloneJSONValue(val any, active map[jsonVisit]bool) (any, bool) {
	switch value := val.(type) {
	case nil, bool, string:
		return value, true
	case json.Number:
		_, err := json.Marshal(value)
		return value, err == nil
	case float64:
		return value, !math.IsInf(value, 0) && !math.IsNaN(value)
	case map[string]any:
		if value == nil {
			return nil, true
		}
		visit := jsonVisit{pointer: reflect.ValueOf(value).Pointer(), kind: reflect.Map}
		if active[visit] {
			return nil, false
		}
		active[visit] = true
		defer delete(active, visit)
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			itemClone, ok := cloneJSONValue(item, active)
			if !ok {
				return nil, false
			}
			cloned[key] = itemClone
		}
		return cloned, true
	case []any:
		if value == nil {
			return nil, true
		}
		visit := jsonVisit{pointer: reflect.ValueOf(value).Pointer(), kind: reflect.Slice, length: len(value)}
		if active[visit] {
			return nil, false
		}
		active[visit] = true
		defer delete(active, visit)
		cloned := make([]any, len(value))
		for i, item := range value {
			itemClone, ok := cloneJSONValue(item, active)
			if !ok {
				return nil, false
			}
			cloned[i] = itemClone
		}
		return cloned, true
	default:
		return nil, false
	}
}

func (j *jsonValues) Bytes() []byte {
	b, _ := j.sj.MarshalJSON()
	return b
}

func (j *jsonValues) Map() map[string]any {
	m, _ := j.sj.Map()
	if m == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(m))
	for key, value := range m {
		result[key] = cloneValue(value)
	}
	return result
}

func (j *jsonValues) Scan(v any) error {
	b, err := j.sj.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (j *jsonValues) String() string {
	return "json"
}

func (j *jsonValue) Bool(def bool) bool {
	b, err := j.Json.Bool()
	if err == nil {
		return b
	}

	str, ok := j.Interface().(string)
	if !ok {
		return def
	}

	b, err = strconv.ParseBool(str)
	if err != nil {
		return def
	}

	return b
}

func (j *jsonValue) Int(def int) int {
	i, err := j.Json.Int()
	if err == nil {
		return i
	}

	str, ok := j.Interface().(string)
	if !ok {
		return def
	}

	i, err = strconv.Atoi(str)
	if err != nil {
		return def
	}

	return i
}

func (j *jsonValue) String(def string) string {
	return j.MustString(def)
}

func (j *jsonValue) Float64(def float64) float64 {
	f, err := j.Json.Float64()
	if err == nil {
		return f
	}

	str, ok := j.Interface().(string)
	if !ok {
		return def
	}

	f, err = strconv.ParseFloat(str, 64)
	if err != nil {
		return def
	}

	return f
}

func (j *jsonValue) Duration(def time.Duration) time.Duration {
	v, err := j.Json.String()
	if err != nil {
		return def
	}

	value, err := time.ParseDuration(v)
	if err != nil {
		return def
	}

	return value
}

func (j *jsonValue) StringSlice(def []string) []string {
	v, err := j.Json.String()
	if err == nil {
		sl := strings.Split(v, ",")
		if len(sl) > 1 {
			return sl
		}
	}
	return slices.Clone(j.MustStringArray(def))
}

func (j *jsonValue) StringMap(def map[string]string) map[string]string {
	m, err := j.Map()
	if err != nil {
		return def
	}

	res := map[string]string{}

	for k, v := range m {
		res[k] = fmt.Sprintf("%v", v)
	}

	return res
}

func (j *jsonValue) Scan(v any) error {
	b, err := j.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (j *jsonValue) Bytes() []byte {
	b, err := j.Json.Bytes()
	if err != nil {
		// try return marshalled
		b, err = j.MarshalJSON()
		if err != nil {
			return []byte{}
		}
		return b
	}
	return bytes.Clone(b)
}

// cloneValue copies the JSON tree; primitive JSON values are immutable.
func cloneValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for key, value := range v {
			m[key] = cloneValue(value)
		}
		return m
	case []any:
		values := make([]any, len(v))
		for i, value := range v {
			values[i] = cloneValue(value)
		}
		return values
	default:
		return v
	}
}
