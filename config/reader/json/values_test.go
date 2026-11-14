package json

import (
	"reflect"
	"testing"

	"github.com/gatework/motor-core/config/source"
)

func TestValues(t *testing.T) {
	emptyStr := ""
	testData := []struct {
		csdata   []byte
		path     []string
		accepter interface{}
		value    interface{}
	}{
		{
			[]byte(`{"foo": "bar", "baz": {"bar": "cat"}}`),
			[]string{"foo"},
			emptyStr,
			"bar",
		},
		{
			[]byte(`{"foo": "bar", "baz": {"bar": "cat"}}`),
			[]string{"baz", "bar"},
			emptyStr,
			"cat",
		},
	}

	for idx, test := range testData {
		values, err := newValues(&source.ChangeSet{
			Data: test.csdata,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = values.Get(test.path...).Scan(&test.accepter)
		if err != nil {
			t.Fatal(err)
		}
		if test.accepter != test.value {
			t.Fatalf("No.%d Expected %v got %v for path %v", idx, test.value, test.accepter, test.path)
		}
	}
}

func TestStructArray(t *testing.T) {
	type T struct {
		Foo string
	}

	emptyTSlice := []T{}

	testData := []struct {
		csdata   []byte
		accepter []T
		value    []T
	}{
		{
			[]byte(`[{"foo": "bar"}]`),
			emptyTSlice,
			[]T{{Foo: "bar"}},
		},
	}

	for idx, test := range testData {
		values, err := newValues(&source.ChangeSet{
			Data: test.csdata,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = values.Get().Scan(&test.accepter)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(test.accepter, test.value) {
			t.Fatalf("No.%d Expected %v got %v", idx, test.value, test.accepter)
		}
	}
}

func TestValuesOwnTheirJSONTree(t *testing.T) {
	values, err := newValues(&source.ChangeSet{Data: []byte(`{"nested":{"name":"original"},"items":["a"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	view := values.Map()
	view["nested"].(map[string]interface{})["name"] = "changed"
	if got := values.Get("nested", "name").String(""); got != "original" {
		t.Fatalf("Map exposed the tree: %q", got)
	}
	snapshot := values.Get("nested")
	input := map[string]string{"name": "next"}
	if err := values.Set(input, "nested"); err != nil {
		t.Fatal(err)
	}
	input["name"] = "changed"
	if got := values.Get("nested", "name").String(""); got != "next" {
		t.Fatalf("Set retained caller data: %q", got)
	}
	var first map[string]string
	if err := snapshot.Scan(&first); err != nil || first["name"] != "original" {
		t.Fatalf("Get did not preserve its snapshot: %v, %v", first, err)
	}
}

func TestSetRejectsInvalidJSONValues(t *testing.T) {
	values, err := newValues(&source.ChangeSet{Data: []byte(`{"value":"original"}`)})
	if err != nil {
		t.Fatal(err)
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, value := range []any{make(chan int), cycle} {
		if err := values.Set(value, "value"); err == nil {
			t.Fatal("Set accepted a value without a JSON representation")
		}
		if got := values.Get("value").String(""); got != "original" {
			t.Fatalf("failed Set changed the value to %q", got)
		}
	}
}
