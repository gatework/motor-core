package config

import (
	"context"
	"time"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/cache"
	redisstore "github.com/gatework/motor-core/storage/redis"
)

// Cache selects Redis when configured, otherwise an in-memory cache.
type Cache struct {
	Redis  *RedisOptions `yaml:"redis" json:"redis"`
	Memory *CacheMemory  `yaml:"memory" json:"memory"`
}

type CacheMemory struct{}

// Open constructs a cache owned by the caller. Close releases its resources,
// including the Redis client created for this instance. Connection failures are
// returned instead of silently falling back to a private in-memory cache.
func (c Cache) Open(ctx context.Context) (storage.Cache, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.Redis == nil {
		return cache.New(cache.Options{SweepInterval: time.Minute}), nil
	}
	opts, err := c.Redis.Options()
	if err != nil {
		return nil, err
	}
	return redisstore.OpenCache(ctx, opts)
}
