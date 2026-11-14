package cache_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage/cache"
)

func BenchmarkMixedLatency(b *testing.B) {
	c := cache.New(cache.Options{SweepInterval: time.Millisecond})
	defer c.Close()
	ctx := context.Background()
	c.Set(ctx, "key", "v", time.Hour)
	var mu sync.Mutex
	samples := []int64{}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		local := make([]int64, 0, 10000)
		i := 0
		for pb.Next() {
			start := time.Now()
			if i%10 == 0 {
				c.Set(ctx, "key", "v", time.Hour)
			} else {
				c.Get(ctx, "key")
			}
			if len(local) < cap(local) {
				local = append(local, time.Since(start).Nanoseconds())
			}
			i++
		}
		mu.Lock()
		samples = append(samples, local...)
		mu.Unlock()
	})
	b.StopTimer()
	slices.Sort(samples)
	if len(samples) > 0 {
		b.ReportMetric(float64(samples[(len(samples)-1)*99/100]), "p99-ns")
	}
}
