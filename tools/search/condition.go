package search

// Clause is a SQL predicate with separately bound values.
type Clause struct {
	SQL  string
	Args []any
}

// Condition preserves the declaration order of filters, including repeated columns.
type Condition struct {
	Where []Clause
	Or    []Clause
	Order []string
	Join  []Join
}

type Join struct {
	SQL       string
	Condition Condition
}
