package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/cache"
	"github.com/gatework/motor-core/storage/queue"
	redisstore "github.com/gatework/motor-core/storage/redis"
)

func TestOpenDefaultsToMemory(t *testing.T) {
	var settings struct {
		Cache Cache
		Queue Queue
	}
	if err := json.Unmarshal([]byte(`{"application":{"mode":"dev"}}`), &settings); err != nil {
		t.Fatal(err)
	}
	c, err := settings.Cache.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.(*cache.Memory); !ok {
		t.Fatalf("cache backend = %T", c)
	}
	q, err := settings.Queue.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Shutdown(context.Background())
	if _, ok := q.(*queue.Memory); !ok {
		t.Fatalf("queue backend = %T", q)
	}
	if !settings.Queue.Empty() {
		t.Fatal("unconfigured queue is not empty")
	}
}

func TestMemorySectionSelectsMemory(t *testing.T) {
	cfg := Queue{Memory: &QueueMemory{PoolSize: 10}}
	q, err := cfg.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Shutdown(context.Background())
	if _, ok := q.(*queue.Memory); !ok {
		t.Fatalf("backend = %T", q)
	}
	if cfg.Empty() {
		t.Fatal("configured queue reported empty")
	}
}

func TestUnreachableRedisIsAnError(t *testing.T) {
	opts := RedisOptions{Addr: "127.0.0.1:1"}
	if _, err := (Cache{Redis: &opts}).Open(t.Context()); err == nil {
		t.Fatal("unreachable Redis cache silently fell back")
	}
	if _, err := (Queue{Redis: &RedisQueue{RedisOptions: opts}}).Open(t.Context()); err == nil {
		t.Fatal("unreachable Redis queue silently fell back")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (Cache{}).Open(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cache cancellation = %v", err)
	}
	if _, err := (Queue{}).Open(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("queue cancellation = %v", err)
	}
}

func TestRedisOptionsNeedsAnAddress(t *testing.T) {
	if _, err := (RedisOptions{}).Options(); err == nil {
		t.Fatal("empty Redis address accepted")
	}
}

func TestURLWinsOverFields(t *testing.T) {
	opts, err := (RedisOptions{URL: "redis://localhost:6380/3", Addr: "127.0.0.1:9999"}).Options()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Addr != "localhost:6380" || opts.DB != 3 {
		t.Fatalf("resolved address = %q db = %d", opts.Addr, opts.DB)
	}
}

func TestRedisSectionSelectsRedis(t *testing.T) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL is not set")
	}
	cfg := Cache{Redis: &RedisOptions{URL: url}}
	first, err := cfg.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := cfg.Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, ok := first.(*redisstore.Cache); !ok {
		t.Fatalf("backend = %T", first)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Get(t.Context(), "config-lifecycle-test"); !errors.Is(err, storage.ErrCacheClosed) {
		t.Fatalf("closed cache returned %v", err)
	}
	// Each open owns its client. Closing one must leave another one usable.
	if err := second.Set(t.Context(), "config-lifecycle-test", "value", 0); err != nil {
		t.Fatal(err)
	}
	defer second.Del(context.Background(), "config-lifecycle-test")
	q, err := (Queue{Redis: &RedisQueue{RedisOptions: RedisOptions{URL: url}}}).Open(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer q.Shutdown(context.Background())
	if _, ok := q.(*redisstore.Queue); !ok {
		t.Fatalf("backend = %T", q)
	}
}
