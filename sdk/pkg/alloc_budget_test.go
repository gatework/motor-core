package pkg

import (
	"testing"

	"gorm.io/gorm"
)

// Both helpers run on every request. Allocation counts are deterministic, so
// these are gates rather than measurements - see the note in
// storage/cache/alloc_budget_test.go.
func TestPkgAllocationBudget(t *testing.T) {
	c := benchContext(true)
	c.Set("db", new(gorm.DB))

	// A type assertion off the context map; nothing is constructed.
	if got := testing.AllocsPerRun(100, func() { _, _ = GetORM(c) }); got > 0 {
		t.Errorf("GetORM allocates %.0f times, budget is 0", got)
	}

	// The failure path is on every request of a misconfigured route, so it is
	// budgeted too - a fresh error per call is a fresh allocation per call.
	missing := benchContext(true)
	if got := testing.AllocsPerRun(100, func() { _, _ = GetORM(missing) }); got > 0 {
		t.Errorf("GetORM on a request without a db allocates %.0f times, budget is 0", got)
	}

	// When a gateway already supplied the correlation id this is a header read.
	if got := testing.AllocsPerRun(100, func() { _ = RequestID(c) }); got > 0 {
		t.Errorf("RequestID with an existing id allocates %.0f times, budget is 0", got)
	}
}
