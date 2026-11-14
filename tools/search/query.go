package search

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const (
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
		return p.(queryPlan)
	}
	p := queryPlan{fields: make([]fieldPlan, 0, t.NumField())}
	for i := 0; i < t.NumField(); i++ {
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
	return actual.(queryPlan)
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

func compileField(tag string, postgres bool) (fieldPlan, error) {
	parts := make(map[string]string, 5)
	for item := range strings.SplitSeq(tag, ";") {
		if item == "" {
			continue
		}
		k, v, ok := strings.Cut(item, ":")
		if !ok {
			return fieldPlan{}, fmt.Errorf("invalid search tag %q", tag)
		}
		switch k {
		case "type", "column", "table", "on", "join":
		default:
			return fieldPlan{}, fmt.Errorf("unknown tag key %q", k)
		}
		parts[k] = v
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
	op := ""
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

func resolveValue(v reflect.Value, postgres bool, out *Condition, active []pointerVisit, joined bool) error {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		if v.Kind() == reflect.Pointer {
			visit := pointerVisit{typ: v.Type(), ptr: v.Pointer()}
			for _, prev := range active {
				if prev == visit {
					return fmt.Errorf("search: cyclic query value of type %s", v.Type())
				}
			}
			active = append(active, visit)
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
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
			if joined {
				return fmt.Errorf("search: nested joins are not supported")
			}
			child := Condition{}
			if err := resolveValue(value, postgres, &child, active, true); err != nil {
				return err
			}
			out.Join = append(out.Join, Join{SQL: f.sql, Condition: child})
			continue
		}
		for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
			if value.IsNil() {
				break
			}
			value = value.Elem()
		}
		if !value.IsValid() {
			continue
		}
		if f.kind == "order" {
			if value.Kind() != reflect.String {
				return fmt.Errorf("search: order must be a string")
			}
			direction := strings.ToLower(value.String())
			if direction != "asc" && direction != "desc" {
				return fmt.Errorf("search: invalid order %q", direction)
			}
			out.Order = append(out.Order, f.sql+" "+direction)
			continue
		}
		clause := Clause{SQL: f.sql}
		if f.kind != "isnull" {
			arg := value.Interface()
			if f.prefix != "" || f.suffix != "" {
				if value.Kind() != reflect.String {
					return fmt.Errorf("search: text filter must be a string")
				}
				arg = f.prefix + value.String() + f.suffix
			}
			if postgres && strings.HasPrefix(f.kind, "json_contains") {
				data, err := json.Marshal(arg)
				if err != nil {
					return fmt.Errorf("search: encode json filter: %w", err)
				}
				arg = string(data)
			}
			clause.Args = []any{arg}
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
