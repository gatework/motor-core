package captcha

import (
	"context"
	"time"

	"github.com/mojocn/base64Captcha"

	"github.com/gatework/motor-core/storage"
)

type cacheStore struct {
	// The third-party Store interface has no context parameter. Its owner
	// explicitly supplies the lifetime of operations at this adapter boundary.
	ctx        context.Context
	cache      storage.Cache
	expiration time.Duration
}

// NewCacheStore adapts cache to the third-party captcha store. ctx supplies the
// operation lifetime absent from that interface. The caller retains ownership
// of cache and must close it when it is no longer needed.
func NewCacheStore(ctx context.Context, cache storage.Cache, expiration time.Duration) base64Captcha.Store {
	return &cacheStore{ctx: ctx, cache: cache, expiration: expiration}
}

func (s *cacheStore) Set(id, value string) error {
	return s.cache.Set(s.ctx, id, value, s.expiration)
}

func (s *cacheStore) Get(id string, clear bool) string {
	value, err := s.cache.Get(s.ctx, id)
	if err != nil {
		return ""
	}
	if clear {
		if err := s.cache.Del(s.ctx, id); err != nil {
			return ""
		}
	}
	return value
}

func (s *cacheStore) Verify(id, answer string, clear bool) bool {
	if id == "" || answer == "" {
		return false
	}
	return s.Get(id, clear) == answer
}
