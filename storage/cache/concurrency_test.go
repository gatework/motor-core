package cache

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The in-memory cache is what a single-instance deployment runs on: leaving the
// redis section unset in settings.yml selects it. Everything here therefore
// exercises it the way a live server does - every operation from many
// goroutines at once - rather than one call at a time.
//
// Run the correctness tests under -race. The throughput benchmarks are meant to
// be read as a ceiling for one process, so run those without it.

const (
	concurrentWriters = 50
	opsPerWriter      = 200
)

func TestMemoryExpireDoesNotRaceWithReaders(t *testing.T) {
	m := New(Options{})
	defer func() { _ = m.Close() }()

	if err := m.Set(context.Background(), "k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup

	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := m.Get(context.Background(), "k"); err != nil {
					t.Error(err)
					return
				}
			}
		}
	}()

	for i := 0; i < 2000; i++ {
		if err := m.Expire(context.Background(), "k", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	readers.Wait()
}

// TestMemoryMixedTrafficIsRaceFree runs the four operations a request path
// actually mixes. It asserts nothing beyond "no race and no error", which is
// what -race is here to check.
func TestMemoryMixedTrafficIsRaceFree(t *testing.T) {
	m := New(Options{SweepInterval: 10 * time.Millisecond})
	defer func() { _ = m.Close() }()

	if err := m.Set(context.Background(), "counter", "0", time.Minute); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < concurrentWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			for j := 0; j < opsPerWriter; j++ {
				if err := m.Set(context.Background(), key, strconv.Itoa(j), time.Minute); err != nil {
					t.Error(err)
					return
				}
				if _, err := m.Get(context.Background(), key); err != nil {
					t.Error(err)
					return
				}
				if _, err := m.Incr(context.Background(), "counter", 1); err != nil {
					t.Error(err)
					return
				}
				if err := m.Expire(context.Background(), key, time.Minute); err != nil {
					t.Error(err)
					return
				}
			}
			if err := m.Del(context.Background(), key); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
}

// --- throughput ---------------------------------------------------------
//
// RunParallel spreads the work over GOMAXPROCS goroutines, so these report what
// one process sustains with every core pushing, not a single-threaded figure.

func BenchmarkMemCacheGet(b *testing.B) {
	ctx := context.Background()
	m := New(Options{SweepInterval: 0})
	defer func() { _ = m.Close() }()
	for i := 0; i < 1024; i++ {
		if err := m.Set(ctx, "k"+strconv.Itoa(i), "v", time.Hour); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, err := m.Get(ctx, "k"+strconv.Itoa(i%1024)); err != nil {
				b.Error(err)
				return
			}
			i++
		}
	})
}

// BenchmarkMemCacheGetHotKey is the worst case: every goroutine reads the same
// key, so they all land on one shard and sharding buys nothing. Keeping it
// alongside the spread version stops the spread number from being read as a
// promise about hot keys.
func BenchmarkMemCacheGetHotKey(b *testing.B) {
	ctx := context.Background()
	m := New(Options{SweepInterval: 0})
	defer func() { _ = m.Close() }()
	if err := m.Set(ctx, "k", "v", time.Hour); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := m.Get(ctx, "k"); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkMemCacheSet(b *testing.B) {
	ctx := context.Background()
	m := New(Options{SweepInterval: 0})
	defer func() { _ = m.Close() }()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if err := m.Set(ctx, "k"+strconv.Itoa(i%64), "v", time.Hour); err != nil {
				b.Error(err)
				return
			}
			i++
		}
	})
}

// BenchmarkMemCacheIncr counts on one key, which is what a rate limiter or a
// failed-login counter does. It is a hot key by nature, so it does not scale
// with shards - the serialisation is the point of the operation.
func BenchmarkMemCacheIncr(b *testing.B) {
	ctx := context.Background()
	m := New(Options{SweepInterval: 0})
	defer func() { _ = m.Close() }()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := m.Incr(ctx, "n", 1); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

// BenchmarkMemCacheIncrSpread counts on many keys - per-user or per-IP
// counters, which is the shape that should scale.
func BenchmarkMemCacheIncrSpread(b *testing.B) {
	ctx := context.Background()
	m := New(Options{SweepInterval: 0})
	defer func() { _ = m.Close() }()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, err := m.Incr(ctx, "n"+strconv.Itoa(i%1024), 1); err != nil {
				b.Error(err)
				return
			}
			i++
		}
	})
}
