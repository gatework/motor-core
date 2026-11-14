package ws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// BenchmarkManagerSend measures completed delivery, including manager admission
// and the peer's network read. Four producers use disjoint clients; extra groups
// expose work that scales with idle membership rather than the target client.
func BenchmarkManagerSend(b *testing.B) {
	for _, groupCount := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("groups=%d", groupCount), func(b *testing.B) {
			benchmarkManagerSend(b, groupCount)
		})
	}
}

func benchmarkManagerSend(b *testing.B, groupCount int) {
	m, err := New()
	if err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- m.Run(ctx) }()
	for {
		m.mu.Lock()
		ready := m.isRunning
		m.mu.Unlock()
		if ready {
			break
		}
		time.Sleep(time.Millisecond)
	}
	upgrader := websocket.Upgrader{}
	accepted := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- conn
		}
	}))
	peers := make([]*websocket.Conn, 0, groupCount)
	received := make([]chan error, groupCount)
	var readers sync.WaitGroup
	defer func() {
		cancel()
		_ = m.Shutdown(context.Background())
		<-runDone
		for _, peer := range peers {
			_ = peer.Close()
		}
		readers.Wait()
		srv.Close()
	}()
	for group := range groupCount {
		peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
		if err != nil {
			b.Fatal(err)
		}
		peers = append(peers, peer)
		if err := m.Connect(ctx, "target", strconv.Itoa(group), <-accepted); err != nil {
			b.Fatal(err)
		}
		received[group] = make(chan error, 1)
		readers.Go(func() {
			for {
				_, _, err := peer.ReadMessage()
				if err != nil {
					return
				}
				received[group] <- nil
			}
		})
	}
	groups := make([]string, groupCount)
	for i := range groups {
		groups[i] = strconv.Itoa(i)
	}
	latencies := make([]time.Duration, b.N)
	failures := make(chan error, 4)
	var producers sync.WaitGroup
	b.ReportAllocs()
	b.ResetTimer()
	started := time.Now()
	for worker := range 4 {
		producers.Go(func() {
			group := worker
			for op := worker; op < b.N; op += 4 {
				before := time.Now()
				if err := m.Send(ctx, "target", groups[group], []byte(`null`)); err != nil {
					failures <- err
					return
				}
				select {
				case <-received[group]:
				case <-time.After(time.Second):
					failures <- fmt.Errorf("group %d did not receive its admitted message", group)
					return
				}
				latencies[op] = time.Since(before)
				group += 4
				if group >= groupCount {
					group = worker
				}
			}
		})
	}
	producers.Wait()
	elapsed := time.Since(started)
	b.StopTimer()
	close(failures)
	for err := range failures {
		b.Fatal(err)
	}
	slices.Sort(latencies)
	b.ReportMetric(float64(latencies[(len(latencies)-1)*99/100].Nanoseconds()), "p99-ns/op")
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "delivered/s")
	b.ReportMetric(1, "delivered/op")
}
