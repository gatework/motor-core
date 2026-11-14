package cache

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// Without a capacity the cache grows until the process dies, and reaching that state
// needs no bug - an unauthenticated endpoint that writes one entry per request
// is enough. These pin the bound and what it costs.

// TestCapacityIsBounded writes far past the capacity with distinct keys, which is
// the shape of the captcha endpoint under load: a fresh key every time, none of
// them read again.
func TestCapacityIsBounded(t *testing.T) {
	ctx := context.Background()
	const capacity = 3200 // 100 per shard
	m := New(Options{MaxEntries: capacity})
	defer func() { _ = m.Close() }()

	for i := range capacity * 10 {
		if err := m.Set(ctx, "k"+strconv.Itoa(i), "v", time.Hour); err != nil {
			t.Fatal(err)
		}
	}

	got := countEntries(m)
	if got > capacity {
		t.Errorf("cache holds %d entries with a capacity of %d", got, capacity)
	}
	// A capacity that evicts far more than it must would make the cache useless.
	if got < capacity/2 {
		t.Errorf("cache holds only %d entries with a capacity of %d; eviction is too eager", got, capacity)
	}
}

// TestEvictionPrefersExpiredEntries checks the cheap option is taken first. A
// shard full of expired entries should need no eviction of live ones.
func TestEvictionPrefersExpiredEntries(t *testing.T) {
	ctx := context.Background()
	const capacity = 3200
	m := New(Options{MaxEntries: capacity})
	defer func() { _ = m.Close() }()

	// Fill with entries that expire almost immediately.
	for i := range capacity {
		if err := m.Set(ctx, "old"+strconv.Itoa(i), "v", time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(20 * time.Millisecond)

	// Now write live entries. They should displace the expired ones.
	const live = 1000
	for i := range live {
		if err := m.Set(ctx, "live"+strconv.Itoa(i), "v", time.Hour); err != nil {
			t.Fatal(err)
		}
	}

	survived := 0
	for i := range live {
		if _, err := m.Get(ctx, "live"+strconv.Itoa(i)); err == nil {
			survived++
		}
	}
	// Expired entries were available to reclaim, so the live writes should
	// almost all still be there. Some loss is possible when writes concentrate
	// on one shard, so this is a floor rather than an equality.
	if survived < live*9/10 {
		t.Errorf("only %d of %d live entries survived; eviction is dropping live "+
			"entries while expired ones remain", survived, live)
	}
}

// TestUnlimitedIsOptOut keeps the escape hatch honest for a caller whose
// keyspace is bounded some other way.
func TestUnlimitedIsOptOut(t *testing.T) {
	ctx := context.Background()
	m := New(Options{MaxEntries: -1})
	defer func() { _ = m.Close() }()

	const n = 5000
	for i := range n {
		if err := m.Set(ctx, "k"+strconv.Itoa(i), "v", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if got := countEntries(m); got != n {
		t.Errorf("unlimited cache holds %d of %d entries", got, n)
	}
}

// TestDefaultConstructorIsBounded is the one that matters in production: the
// constructors everything actually calls must carry the capacity, not just the
// options form nobody uses.
func TestDefaultConstructorIsBounded(t *testing.T) {
	for name, m := range map[string]*Memory{
		"with background collection":    New(Options{SweepInterval: time.Minute}),
		"without background collection": New(Options{SweepInterval: 0}),
	} {
		if m.maxPerShard <= 0 {
			t.Errorf("%s builds an unbounded cache", name)
		}
		_ = m.Close()
	}
}

// BenchmarkSetAtCapacity is the steady state of a cache that has filled up:
// every write adds a key nobody will read again, so every write has to evict.
// This is the captcha endpoint under load, and the shape that makes eviction a
// per-write cost rather than an occasional one.
//
// It is here to keep that cost flat. An eviction that scanned the shard for
// expired entries instead of sampling made this grow with shard size.
func BenchmarkSetAtCapacity(b *testing.B) {
	ctx := context.Background()
	const capacity = 32 * 4096 // 4096 per shard
	m := New(Options{MaxEntries: capacity})
	defer func() { _ = m.Close() }()

	for i := range capacity {
		if err := m.Set(ctx, "fill"+strconv.Itoa(i), "v", time.Hour); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if err := m.Set(ctx, "new"+strconv.Itoa(i), "v", time.Hour); err != nil {
			b.Fatal(err)
		}
	}
}
