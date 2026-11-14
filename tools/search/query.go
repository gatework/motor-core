package search

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// Supported query tags and SQL dialect names.
const (
	// FromQueryTag names the struct tag used to define search filters.
	FromQueryTag = "search"
	MySQL        = "mysql"
	Postgres     = "postgres"
)

type planKey struct {
	typ      reflect.Type
	postgres bool
}
type fieldPlan struct {
	index                     int
	kind, sql, prefix, suffix string
}
type queryPlan struct {
	fields []fieldPlan
	err    error
}

// Plans contain only immutable type metadata, never a request's reflect.Value.
var plans sync.Map

func typePlan(t reflect.Type, postgres bool) queryPlan {
	key := planKey{typ: t, postgres: postgres}
	if p, ok := plans.Load(key); ok {
		cached, _ := p.(queryPlan)
		return cached
	}
	p := queryPlan{fields: make([]fieldPlan, 0, t.NumField())}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag, tagged := f.Tag.Lookup(FromQueryTag)
		if tag == "-" {
			continue
		}
		if !tagged {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct || ft.Kind() == reflect.Interface {
				p.fields = append(p.fields, fieldPlan{index: i, kind: "child"})
			}
			continue
		}
		fp, err := compileField(tag, postgres)
		if err != nil {
			p.err = fmt.Errorf("search: %s.%s: %w", t, f.Name, err)
			break
		}
		fp.index = i
		p.fields = append(p.fields, fp)
	}
	actual, _ := plans.LoadOrStore(key, p)
	cached, _ := actual.(queryPlan)
	return cached
}

func identifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func parseTag(tag string) (map[string]string, error) {
	parts := make(map[string]string, 5)
	for item := range strings.SplitSeq(tag, ";") {
		if item == "" {
			continue
		}
		k, v, ok := strings.Cut(item, ":")
		if !ok {
			return nil, fmt.Errorf("invalid search tag %q", tag)
		}
		switch k {
		case "type", "column", "table", "on", "join":
		default:
			return nil, fmt.Errorf("unknown tag key %q", k)
		}
		parts[k] = v
	}
	return parts, nil
}

func compileField(tag string, postgres bool) (fieldPlan, error) {
	parts, err := parseTag(tag)
	if err != nil {
		return fieldPlan{}, err
	}
	p := fieldPlan{kind: parts["type"]}
	quote := func(s string) string {
		if postgres {
			return s
		}
		return "`" + s + "`"
	}
	table := parts["table"]
	if !identifier(table) {
		return p, fmt.Errorf("invalid table %q", table)
	}
	if p.kind == "left" {
		join := parts["join"]
		a, b, ok := strings.Cut(parts["on"], ":")
		if !ok || !identifier(a) || !identifier(b) || !identifier(join) {
			return p, fmt.Errorf("invalid join tag %q", tag)
		}
		p.sql = "left join " + quote(join) + " on " + quote(join) + "." + quote(a) + " = " + quote(table) + "." + quote(b)
		return p, nil
	}
	column := parts["column"]
	if !identifier(column) {
		return p, fmt.Errorf("invalid column %q", column)
	}
	col := quote(table) + "." + quote(column)
	return compilePredicate(p, col, postgres)
}

func compilePredicate(p fieldPlan, col string, postgres bool) (fieldPlan, error) {
	var op string
	switch p.kind {
	case "exact", "iexact":
		op = " = ?"
	case "glt":
		op = " <> ?"
	case "gt":
		op = " > ?"
	case "gte":
		op = " >= ?"
	case "lt":
		op = " < ?"
	case "lte":
		op = " <= ?"
	case "in":
		op = " in (?)"
	case "contains", "icontains", "startswith", "istartswith", "endswith", "iendswith":
		op = " like ?"
		if postgres && strings.HasPrefix(p.kind, "i") {
			op = " ilike ?"
		}
		if strings.Contains(p.kind, "contains") || strings.Contains(p.kind, "endswith") {
			p.prefix = "%"
		}
		if strings.Contains(p.kind, "contains") || strings.Contains(p.kind, "startswith") {
			p.suffix = "%"
		}
	case "isnull":
		op = " is null"
	case "order":
		p.sql = col
		return p, nil
	case "json_contains", "json_contains_or_null":
		p.sql = jsonSQL(postgres, col, p.kind == "json_contains_or_null")
		return p, nil
	default:
		return p, fmt.Errorf("unsupported search type %q", p.kind)
	}
	p.sql = col + op
	return p, nil
}

type pointerVisit struct {
	typ reflect.Type
	ptr uintptr
}

// Resolve returns conditions in DTO field order. Malformed tags and cyclic
// values are errors, so callers never execute a partially resolved filter.
func Resolve(driver string, q any) (Condition, error) {
	var out Condition
	err := resolveValue(reflect.ValueOf(q), driver == Postgres, &out, nil, false)
	if err != nil {
		return Condition{}, err
	}
	return out, nil
}

func unwrapValue(v reflect.Value, active []pointerVisit) (reflect.Value, []pointerVisit, error) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, active, nil
		}
		if v.Kind() == reflect.Pointer {
			visit := pointerVisit{typ: v.Type(), ptr: v.Pointer()}
			if slices.Contains(active, visit) {
				return reflect.Value{}, active, fmt.Errorf("search: cyclic query value of type %s", v.Type())
			}
			active = append(active, visit)
		}
		v = v.Elem()
	}
	return v, active, nil
}

func resolveValue(v reflect.Value, postgres bool, out *Condition, active []pointerVisit, joined bool) error {
	v, active, err := unwrapValue(v, active)
	if err != nil || v.Kind() != reflect.Struct {
		return err
	}
	p := typePlan(v.Type(), postgres)
	if p.err != nil {
		return p.err
	}
	for _, f := range p.fields {
		value := v.Field(f.index)
		if f.kind == "child" {
			if err := resolveValue(value, postgres, out, active, joined); err != nil {
				return err
			}
			continue
		}
		if value.IsZero() {
			continue
		}
		if f.kind == "left" {
			join, err := resolveJoin(f, value, postgres, active, joined)
			if err != nil {
				return err
			}
			out.Join = append(out.Join, join)
			continue
		}
		value, _, err = unwrapValue(value, active)
		if err != nil {
			return err
		}
		if !value.IsValid() {
			continue
		}
		if f.kind == "order" {
			order, err := resolveOrder(f.sql, value)
			if err != nil {
				return err
			}
			out.Order = append(out.Order, order)
			continue
		}
		clause, err := resolveClause(f, value, postgres)
		if err != nil {
			return err
		}
		out.Where = append(out.Where, clause)
	}
	return nil
}

func jsonSQL(postgres bool, col string, nullable bool) string {
	contains := "JSON_CONTAINS(" + col + ", JSON_QUOTE(?))"
	empty := "JSON_ARRAY()"
	if postgres {
		contains = col + " @> ?::jsonb"
		empty = "'[]'::jsonb"
	}
	if nullable {
		return "(" + col + " IS NULL OR " + col + " = " + empty + " OR " + contains + ")"
	}
	return contains
}

// BuildJSONContainsClause accepts a trusted column reference, never user input.
func BuildJSONContainsClause(driver, column string, value any) (string, []any, error) {
	return jsonClause(driver, column, value, false)
}

// BuildJSONContainsOrNullClause builds a JSON containment predicate that also accepts null and empty arrays.
func BuildJSONContainsOrNullClause(driver, column string, value any) (string, []any, error) {
	return jsonClause(driver, column, value, true)
}

func jsonClause(driver, column string, value any, nullable bool) (string, []any, error) {
	if driver == Postgres {
		b, err := json.Marshal(value)
		if err != nil {
			return "", nil, fmt.Errorf("search: encode json filter: %w", err)
		}
		value = string(b)
	}
	return jsonSQL(driver == Postgres, column, nullable), []any{value}, nil
}

func resolveClause(f fieldPlan, value reflect.Value, postgres bool) (Clause, error) {
	clause := Clause{SQL: f.sql}
	if f.kind != "isnull" {
		arg := value.Interface()
		if f.prefix != "" || f.suffix != "" {
			if value.Kind() != reflect.String {
				return Clause{}, fmt.Errorf("search: text filter must be a string")
			}
			arg = f.prefix + value.String() + f.suffix
		}
		if postgres && strings.HasPrefix(f.kind, "json_contains") {
			data, err := json.Marshal(arg)
			if err != nil {
				return Clause{}, fmt.Errorf("search: encode json filter: %w", err)
			}
			arg = string(data)
		}
		clause.Args = []any{arg}
	}
	return clause, nil
}

func resolveOrder(sql string, value reflect.Value) (string, error) {
	if value.Kind() != reflect.String {
		return "", fmt.Errorf("search: order must be a string")
	}
	direction := strings.ToLower(value.String())
	if direction != "asc" && direction != "desc" {
		return "", fmt.Errorf("search: invalid order %q", direction)
	}
	return sql + " " + direction, nil
}

func resolveJoin(f fieldPlan, value reflect.Value, postgres bool, active []pointerVisit, joined bool) (Join, error) {
	if joined {
		return Join{}, fmt.Errorf("search: nested joins are not supported")
	}
	var child Condition
	if err := resolveValue(value, postgres, &child, active, true); err != nil {
		return Join{}, err
	}
	return Join{SQL: f.sql, Condition: child}, nil
}
