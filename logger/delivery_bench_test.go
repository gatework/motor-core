package logger

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type countingOutput struct{ written atomic.Uint64 }

func (w *countingOutput) Write(p []byte) (int, error) { w.written.Add(1); return len(p), nil }

// BenchmarkDelivery includes draining accepted entries in its elapsed time.
// Drop admission cost is never reported as successful sink throughput.
func BenchmarkDelivery(b *testing.B) {
	for _, policy := range []string{"sync", "block", "drop"} {
		b.Run(policy, func(b *testing.B) {
			out := &countingOutput{}
			base := NewLogger(WithOutput(out))
			var log Logger = base
			var async *Async
			if policy != "sync" {
				async = NewAsyncLogger(base, AsyncConfig{BufferSize: 1024, FlushInterval: time.Millisecond, DropPolicy: policy})
				log = async
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				log.Log(ctx, InfoLevel, "entry")
			}
			var dropped uint64
			if async != nil {
				if err := async.Close(); err != nil {
					b.Fatal(err)
				}
				_, dropped = async.GetStats()
			} else if err := base.Close(); err != nil {
				b.Fatal(err)
			}
			b.StopTimer()
			written := out.written.Load()
			if written+dropped != uint64(b.N) {
				b.Fatalf("written %d + dropped %d != attempted %d", written, dropped, b.N)
			}
			b.ReportMetric(float64(written)/float64(b.N), "written/op")
			b.ReportMetric(float64(dropped)/float64(b.N), "dropped/op")
			b.ReportMetric(float64(written)/b.Elapsed().Seconds(), "written/s")
		})
	}
}
