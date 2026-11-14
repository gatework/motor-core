package redis

import (
	"context"
	"errors"

	goredis "github.com/redis/go-redis/v9"
)

// dial validates the connection before handing ownership to a backend.
func dial(ctx context.Context, opts *goredis.Options) (*goredis.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts == nil {
		return nil, errors.New("storage: nil redis options")
	}
	ownedOpts := *opts
	ownedOpts.ContextTimeoutEnabled = true
	client := goredis.NewClient(&ownedOpts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
