package captcha

import (
	"testing"
	"time"

	"github.com/gatework/motor-core/storage/cache"
)

// Verify runs on every login attempt. Generation is allocation-heavy by nature
// - it renders a PNG - but verification is a store read and a comparison, and
// there is no reason for it to allocate at all.
//
// Allocation counts are deterministic; see the note in
// storage/cache/alloc_budget_test.go.
func TestVerifyAllocationBudget(t *testing.T) {
	c := cache.New(cache.Options{})
	defer func() { _ = c.Close() }()
	store := NewCacheStore(t.Context(), c, 600*time.Second)
	SetStore(store)

	if err := store.Set("budget-id", "1234"); err != nil {
		t.Fatal(err)
	}
	if !Verify("budget-id", "1234", false) {
		t.Fatal("setup failed: the answer does not verify")
	}

	got := testing.AllocsPerRun(100, func() {
		Verify("budget-id", "1234", false)
	})
	if got > 0 {
		t.Errorf("Verify allocates %.0f times, budget is 0", got)
	}
}
