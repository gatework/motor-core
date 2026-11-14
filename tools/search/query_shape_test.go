package search

import "testing"

func TestResolveSearchQueryAcceptsPointerAndSkipsScalarFields(t *testing.T) {
	type query struct {
		Page int
		Name string `search:"type:exact;column:name;table:users"`
	}
	for _, q := range []any{query{Page: 1, Name: "alice"}, &query{Page: 1, Name: "alice"}} {
		cond := mustResolve(MySQL, q)
		if len(cond.Where) != 1 {
			t.Fatalf("conditions=%v", cond.Where)
		}
	}
}
func TestResolveSearchQueryIgnoresNonStructInput(t *testing.T) {
	for _, q := range []any{nil, (*struct{})(nil), 42, "text"} {
		cond := mustResolve(MySQL, q)
		if len(cond.Where) != 0 {
			t.Fatalf("conditions=%v", cond.Where)
		}
	}
}

func TestResolveRejectsPointerCycle(t *testing.T) {
	type query struct {
		Next *query
		Name string `search:"type:exact;column:name;table:users"`
	}
	q := &query{Name: "alice"}
	q.Next = q
	if _, err := Resolve(MySQL, q); err == nil {
		t.Fatal("cyclic query accepted")
	}
}

func TestResolveRejectsMalformedJoinAndPreservesRepeatedColumns(t *testing.T) {
	malformed := struct {
		Child struct{ X string } `search:"type:left;table:users;join:roles;on:id"`
	}{}
	if _, err := Resolve(MySQL, malformed); err == nil {
		t.Fatal("malformed join accepted")
	}
	query := struct {
		First  string `search:"type:exact;column:name;table:users"`
		Second string `search:"type:exact;column:name;table:users"`
	}{First: "a", Second: "b"}
	cond, err := Resolve(MySQL, query)
	if err != nil || len(cond.Where) != 2 || cond.Where[0].Args[0] != "a" || cond.Where[1].Args[0] != "b" {
		t.Fatalf("conditions=%v error=%v", cond, err)
	}
}
