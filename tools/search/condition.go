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
	// Join pairs a join clause with the filters resolved for that join.
	Join []Join
}

// Join associates a SQL join with its resolved predicates.
type Join struct {
	SQL       string
	Condition Condition
}
