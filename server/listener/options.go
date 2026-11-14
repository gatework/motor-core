package listener

import (
	"errors"
	"net/http"
	"time"
)

const (
	defaultReadHeaderTimeout = 10 * time.Second
	defaultIdleTimeout       = time.Minute
)

// Option configures a listener before it starts.
type Option func(*options) error

type options struct {
	addr, certFile, keyFile string
	handler                 http.Handler
	startedHook             func()
	endHook                 func()
	readHeaderTimeout       time.Duration
	idleTimeout             time.Duration
}

func setDefaultOption() options {
	return options{
		addr:              ":8080",
		readHeaderTimeout: defaultReadHeaderTimeout,
		idleTimeout:       defaultIdleTimeout,
		handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
}

// WithEndHook runs f after the listener has drained or closed its connections.
func WithEndHook(f func()) Option {
	return func(o *options) error { o.endHook = f; return nil }
}

// WithStartedHook runs f after the listening socket is bound.
func WithStartedHook(f func()) Option {
	return func(o *options) error { o.startedHook = f; return nil }
}

// WithAddr sets the TCP listening address.
func WithAddr(addr string) Option {
	return func(o *options) error {
		if addr == "" {
			return errors.New("listener: address is required")
		}
		o.addr = addr
		return nil
	}
}

// WithHandler supplies the HTTP handler owned by the host.
func WithHandler(handler http.Handler) Option {
	return func(o *options) error {
		if handler == nil {
			return errors.New("listener: handler is required")
		}
		o.handler = handler
		return nil
	}
}

// WithCert sets the TLS certificate path; WithKey must also be supplied.
func WithCert(path string) Option {
	return func(o *options) error { o.certFile = path; return nil }
}

// WithKey sets the TLS private-key path; WithCert must also be supplied.
func WithKey(path string) Option {
	return func(o *options) error { o.keyFile = path; return nil }
}

// WithReadHeaderTimeout bounds header parsing without timing request bodies.
// Zero selects the default; negative values are invalid.
func WithReadHeaderTimeout(timeout time.Duration) Option {
	return func(o *options) error {
		if timeout < 0 {
			return errors.New("listener: header timeout must be nonnegative")
		}
		o.readHeaderTimeout = timeout
		if timeout == 0 {
			o.readHeaderTimeout = defaultReadHeaderTimeout
		}
		return nil
	}
}

// WithIdleTimeout bounds keep-alive waiting between requests, not active streams.
// Zero selects the default; negative values are invalid.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(o *options) error {
		if timeout < 0 {
			return errors.New("listener: idle timeout must be nonnegative")
		}
		o.idleTimeout = timeout
		if timeout == 0 {
			o.idleTimeout = defaultIdleTimeout
		}
		return nil
	}
}
