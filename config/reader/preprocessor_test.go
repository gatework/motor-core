package reader

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReplaceEnvVars(t *testing.T) {
	t.Setenv("myBar", "cat")
	t.Setenv("MYBAR", "cat")
	t.Setenv("my_Bar", "cat")
	t.Setenv("myBar_", "cat")

	testData := []struct {
		expected string
		data     []byte
	}{
		// Right use cases
		{
			`{"foo": "bar", "baz": {"bar": "cat"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${myBar}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "cat"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${MYBAR}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "cat"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${my_Bar}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "cat"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${myBar_}"}}`),
		},
		// Wrong use cases
		{
			`{"foo": "bar", "baz": {"bar": "${myBar-}"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${myBar-}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "${}"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "$sss}"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "$sss}"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "${sss"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "${sss"}}`),
		},
		{
			`{"foo": "bar", "baz": {"bar": "{something}"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "{something}"}}`),
		},
		// Use cases without replace env vars
		{
			`{"foo": "bar", "baz": {"bar": "cat"}}`,
			[]byte(`{"foo": "bar", "baz": {"bar": "cat"}}`),
		},
	}

	for _, test := range testData {
		res, err := ReplaceEnvVars(test.data)
		if err != nil {
			t.Fatal(err)
		}
		var want any
		if err := json.Unmarshal([]byte(test.expected), &want); err != nil {
			if string(res) != test.expected {
				t.Fatalf("Expected %s got %s", test.expected, res)
			}
			continue
		}
		var got any
		if err := json.Unmarshal(res, &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Expected %s got %s, error %v", test.expected, res, err)
		}
	}
}

func TestReplaceEnvVarsKeepsEnvironmentValuesAsStrings(t *testing.T) {
	secret := "a\"b\\c\n\t"
	t.Setenv("CONFIG_SECRET", secret)
	t.Setenv("CONFIG_EMPTY", "")
	data := []byte(`{"value":"prefix:${CONFIG_SECRET}","${CONFIG_SECRET}":"key unchanged","nested":["${CONFIG_SECRET}","${CONFIG_EMPTY}","${bad-name}"],"large":9007199254740993,"decimal":0.123456789012345678901234567890}`)
	result, err := ReplaceEnvVars(data)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Value   string
		Nested  []string
		Large   json.RawMessage
		Decimal json.RawMessage
	}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("environment substitution broke JSON: %v", err)
	}
	if got.Value != "prefix:"+secret || len(got.Nested) != 3 || got.Nested[0] != secret || got.Nested[1] != "" || got.Nested[2] != "${bad-name}" {
		t.Fatalf("string values changed unexpectedly: %#v", got)
	}
	if string(got.Large) != "9007199254740993" || string(got.Decimal) != "0.123456789012345678901234567890" {
		t.Fatalf("numeric precision lost: %s, %s", got.Large, got.Decimal)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(result, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["${CONFIG_SECRET}"]; !ok {
		t.Fatal("environment expansion changed a JSON object key")
	}
}
