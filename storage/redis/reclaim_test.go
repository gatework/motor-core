package redis

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gatework/motor-core/storage"
	goredis "github.com/redis/go-redis/v9"
)

type pendingPageClient struct {
	goredis.UniversalClient
	pending []goredis.XPendingExt
	starts  []string
	claims  []string
}

func (c *pendingPageClient) XPendingExt(ctx context.Context, args *goredis.XPendingExtArgs) *goredis.XPendingExtCmd {
	c.starts = append(c.starts, args.Start)
	page := make([]goredis.XPendingExt, 0, args.Count)
	for _, item := range c.pending {
		if args.Start != "-" && item.ID <= strings.TrimPrefix(args.Start, "(") {
			continue
		}
		page = append(page, item)
		if int64(len(page)) == args.Count {
			break
		}
	}
	cmd := goredis.NewXPendingExtCmd(ctx)
	cmd.SetVal(page)
	return cmd
}

func (c *pendingPageClient) XClaim(ctx context.Context, args *goredis.XClaimArgs) *goredis.XMessageSliceCmd {
	messages := make([]goredis.XMessage, 0, len(args.Messages))
	for _, id := range args.Messages {
		c.claims = append(c.claims, id)
		messages = append(messages, goredis.XMessage{ID: id})
	}
	cmd := goredis.NewXMessageSliceCmd(ctx)
	cmd.SetVal(messages)
	return cmd
}

func (c *pendingPageClient) XAck(ctx context.Context, _ string, _ string, _ ...string) *goredis.IntCmd {
	return goredis.NewIntResult(1, nil)
}

func TestReclaimAdvancesPastExhaustedEntries(t *testing.T) {
	client := &pendingPageClient{pending: []goredis.XPendingExt{
		{ID: "1-0", RetryCount: 3}, {ID: "2-0", RetryCount: 3},
		{ID: "3-0", RetryCount: 1}, {ID: "4-0", RetryCount: 3},
		{ID: "5-0", RetryCount: 1},
	}}
	q := newQueue(client, false, QueueOptions{Batch: 2, MaxAttempts: 3})
	var attempts []int
	q.handlers["orders"] = func(_ context.Context, msg storage.Message) error {
		attempts = append(attempts, msg.Attempts)
		return nil
	}
	for range 3 {
		q.reclaim(t.Context(), "orders")
	}
	if !reflect.DeepEqual(client.claims, []string{"3-0", "5-0"}) {
		t.Fatalf("claimed %v, want later eligible messages after the exhausted prefix", client.claims)
	}
	if !reflect.DeepEqual(attempts, []int{2, 2}) {
		t.Fatalf("delivery attempts = %v, want [2 2]", attempts)
	}
	q.reclaim(t.Context(), "orders")
	if !reflect.DeepEqual(client.starts, []string{"-", "(2-0", "(4-0", "-"}) {
		t.Fatalf("scan starts = %v, want bounded pages and wraparound", client.starts)
	}
}

func TestReclaimCursorSurvivesDeletedBoundary(t *testing.T) {
	client := &pendingPageClient{pending: []goredis.XPendingExt{
		{ID: "1-0", RetryCount: 3}, {ID: "2-0", RetryCount: 3},
	}}
	q := newQueue(client, false, QueueOptions{Batch: 2, MaxAttempts: 3})
	q.reclaim(t.Context(), "orders")
	client.pending = nil
	q.reclaim(t.Context(), "orders")
	q.reclaim(t.Context(), "orders")
	if !reflect.DeepEqual(client.starts, []string{"-", "(2-0", "-"}) {
		t.Fatalf("scan starts after deleting the boundary = %v", client.starts)
	}
}
