package config

import (
	"context"
	"errors"
	"time"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/queue"
	redisstore "github.com/gatework/motor-core/storage/redis"
)

// Queue selects Redis when configured, otherwise an in-memory queue.
type Queue struct {
	Redis  *RedisQueue  `yaml:"redis" json:"redis"`
	Memory *QueueMemory `yaml:"memory" json:"memory"`
}

// QueueMemory configures buffered messages per in-process topic.
type QueueMemory struct{ PoolSize uint }

// RedisQueue configures the Redis streams backend and redelivery policy.
type RedisQueue struct {
	RedisOptions        `yaml:",inline"`
	Group               string `yaml:"group" json:"group"`
	KeyPrefix           string `yaml:"key_prefix" json:"key_prefix"`
	MaxAttempts         int    `yaml:"max_attempts" json:"max_attempts"`
	ClaimMinIdleSeconds int    `yaml:"claim_min_idle_seconds" json:"claim_min_idle_seconds"`
}

// Empty reports whether either queue backend was explicitly configured.
func (q Queue) Empty() bool { return q.Memory == nil && q.Redis == nil }

// Open returns a caller-owned queue. Shutdown releases its consumers and the
// Redis client created by this call. Run starts consumption on either backend.
func (q Queue) Open(ctx context.Context) (storage.Queue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Redis == nil {
		var size int
		if q.Memory != nil {
			if q.Memory.PoolSize > ^uint(0)>>1 {
				return nil, errors.New("config: queue pool size overflows int")
			}
			size = int(q.Memory.PoolSize)
		}
		return queue.New(size), nil
	}
	opts, err := q.Redis.Options()
	if err != nil {
		return nil, err
	}
	return redisstore.OpenQueue(ctx, opts, redisstore.QueueOptions{
		Group: q.Redis.Group, KeyPrefix: q.Redis.KeyPrefix, MaxAttempts: q.Redis.MaxAttempts,
		ClaimMinIdle: time.Duration(q.Redis.ClaimMinIdleSeconds) * time.Second,
	})
}
