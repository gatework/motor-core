package captcha

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage/cache"
)

func TestCacheStore(t *testing.T) {
	backend := cache.New(cache.Options{})
	t.Cleanup(func() { _ = backend.Close() })
	store := NewCacheStore(t.Context(), backend, time.Minute)
	if err := store.Set("captcha-id", "1234"); err != nil {
		t.Fatal(err)
	}
	if got := store.Get("captcha-id", false); got != "1234" {
		t.Fatalf("Get returned %q, want 1234", got)
	}
	if store.Verify("", "", false) || store.Verify("missing", "", false) {
		t.Fatal("an absent captcha and empty answer must fail verification")
	}
	if !store.Verify("captcha-id", "1234", true) {
		t.Fatal("the stored answer did not verify")
	}
	if store.Verify("captcha-id", "1234", false) {
		t.Fatal("clear left the answer available for replay")
	}
}

func TestCacheStoreExpiryAndCancellation(t *testing.T) {
	backend := cache.New(cache.Options{})
	t.Cleanup(func() { _ = backend.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	store := NewCacheStore(ctx, backend, 10*time.Millisecond)
	if err := store.Set("short", "1234"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)
	if store.Verify("short", "1234", false) {
		t.Fatal("an expired captcha verified")
	}
	cancel()
	if err := store.Set("cancelled", "1234"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Set after cancellation: %v", err)
	}
	if store.Verify("cancelled", "1234", false) {
		t.Fatal("a cancelled store must fail closed")
	}
}

func BenchmarkSetCollect(b *testing.B) {
	backend := cache.New(cache.Options{})
	b.Cleanup(func() { _ = backend.Close() })
	store := NewCacheStore(b.Context(), backend, time.Minute)
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	b.ResetTimer()
	for b.Loop() {
		for _, id := range ids {
			if err := store.Set(id, "fdskfew9832232r"); err != nil {
				b.Fatal(err)
			}
		}
	}
}
