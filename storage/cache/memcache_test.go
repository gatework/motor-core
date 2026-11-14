package cache_test

import (
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/cache"
	"github.com/gatework/motor-core/storage/cachetest"
)

func TestMemCacheConformance(t *testing.T) {
	cachetest.Run(t, func(t *testing.T) storage.Cache {
		// No sweeper: the suite asserts lazy expiry, which must hold on its own.
		return cache.New(cache.Options{SweepInterval: 0})
	})
}

func TestMemCacheWithSweeperConformance(t *testing.T) {
	cachetest.Run(t, func(t *testing.T) storage.Cache {
		return cache.New(cache.Options{SweepInterval: time.Minute})
	})
}
