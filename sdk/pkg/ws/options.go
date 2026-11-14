package ws

import (
	"errors"
	"time"
)

const (
	defaultMaxMessageBytes int64 = 1 << 20
	defaultWriteTimeout          = 10 * time.Second
)

// Option configures connection resource limits before the manager starts.
type Option func(*options) error

type options struct {
	maxMessageBytes int64
	writeTimeout    time.Duration
}

func defaultOptions() options {
	return options{maxMessageBytes: defaultMaxMessageBytes, writeTimeout: defaultWriteTimeout}
}

// WithMaxMessageBytes bounds each incoming message, including fragmented frames.
// Zero selects the default; negative values are invalid.
func WithMaxMessageBytes(limit int64) Option {
	return func(o *options) error {
		if limit < 0 {
			return errors.New("ws: message limit must be nonnegative")
		}
		o.maxMessageBytes = limit
		if limit == 0 {
			o.maxMessageBytes = defaultMaxMessageBytes
		}
		return nil
	}
}

// WithWriteTimeout bounds one network write before disconnecting a stalled peer.
// Zero selects the default; negative values are invalid.
func WithWriteTimeout(timeout time.Duration) Option {
	return func(o *options) error {
		if timeout < 0 {
			return errors.New("ws: write timeout must be nonnegative")
		}
		o.writeTimeout = timeout
		if timeout == 0 {
			o.writeTimeout = defaultWriteTimeout
		}
		return nil
	}
}
