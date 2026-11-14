/*
 * @Author: lwnmengjing
 * @Date: 2021/6/8 2:04 下午
 * @Last Modified by: lwnmengjing
 * @Last Modified time: 2021/6/8 2:04 下午
 */

package listener

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server owns a listening socket and its request-draining lifecycle.
type Server struct {
	name  string
	mutex sync.Mutex
	opts  options
	run   *serverRun
}

type serverRun struct {
	stop        chan struct{}
	done        chan struct{}
	cancelStart context.CancelFunc
	stopOnce    sync.Once
	err         error
}

func (r *serverRun) stopAccepting() {
	r.stopOnce.Do(func() {
		r.cancelStart()
		close(r.stop)
	})
}

// New constructs a listener with bounded headers and idle keep-alive waits.
func New(name string, opts ...Option) (*Server, error) {
	if name == "" {
		return nil, errors.New("listener: name is required")
	}
	s := &Server{name: name, opts: setDefaultOption()}
	if err := s.Options(opts...); err != nil {
		return nil, err
	}
	return s, nil
}

// NewMetrics constructs a metrics listener on port 3000 unless overridden.
func NewMetrics(opts ...Option) (*Server, error) {
	h := http.NewServeMux()
	h.Handle("/metrics", promhttp.Handler())
	defaults := []Option{WithAddr(":3000"), WithHandler(h)}
	return New("metrics", append(defaults, opts...)...)
}

// NewHealthz constructs a health listener on port 4000 unless overridden.
func NewHealthz(opts ...Option) (*Server, error) {
	return newProbe("healthz", ":4000", "/healthz", opts)
}

// NewReadyz constructs a readiness listener on port 2000 unless overridden.
func NewReadyz(opts ...Option) (*Server, error) {
	return newProbe("readyz", ":2000", "/readyz", opts)
}

func newProbe(name, addr, path string, opts []Option) (*Server, error) {
	h := http.NewServeMux()
	h.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defaults := []Option{WithAddr(addr), WithHandler(h)}
	return New(name, append(defaults, opts...)...)
}

// Options validates settings for the next Run. Invalid changes are not applied.
func (e *Server) Options(opts ...Option) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	next := e.opts
	for _, option := range opts {
		if option == nil {
			return errors.New("listener: option is required")
		}
		if err := option(&next); err != nil {
			return err
		}
	}
	if (next.certFile == "") != (next.keyFile == "") {
		return errors.New("listener: certificate and key must be supplied together")
	}
	e.opts = next
	return nil
}

func (e *Server) String() string {
	return e.name
}

// Run binds before reporting readiness and owns request draining until completion.
func (e *Server) Run(ctx context.Context) (runErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.mutex.Lock()
	if e.run != nil {
		e.mutex.Unlock()
		return errors.New("listener: already running")
	}
	startCtx, cancelStart := context.WithCancel(ctx)
	run := &serverRun{stop: make(chan struct{}), done: make(chan struct{}), cancelStart: cancelStart}
	e.run = run
	opts := e.opts
	e.mutex.Unlock()
	defer cancelStart()
	defer func() {
		e.mutex.Lock()
		run.err = runErr
		e.run = nil
		close(run.done)
		e.mutex.Unlock()
	}()

	l, tlsConfig, err := listen(startCtx, opts)
	if err != nil {
		select {
		case <-run.stop:
			return nil
		default:
			return err
		}
	}
	defer l.Close()
	srv := &http.Server{
		Handler:           opts.handler,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: opts.readHeaderTimeout,
		IdleTimeout:       opts.idleTimeout,
	}
	srv.TLSConfig = tlsConfig
	serving := make(chan error, 1)
	go func() { serving <- serve(srv, l) }()
	if opts.endHook != nil {
		defer opts.endHook()
	}
	if opts.startedHook != nil {
		opts.startedHook()
	}

	var serveErr error
	serveFinished := false
	forceBudget := false
	select {
	case serveErr = <-serving:
		serveFinished = true
		forceBudget = !errors.Is(serveErr, http.ErrServerClosed)
	case <-run.stop:
	case <-ctx.Done():
	}
	// The run owns the drain. A Shutdown caller's deadline only ends its wait,
	// so a later caller can continue waiting for the same active requests.
	drained := make(chan error, 1)
	go func() { drained <- srv.Shutdown(context.WithoutCancel(ctx)) }()
	runErr = waitForDrain(ctx, srv, drained, forceBudget)
	if !serveFinished {
		serveErr = <-serving
	}
	if !errors.Is(serveErr, http.ErrServerClosed) {
		runErr = errors.Join(runErr, serveErr)
	}
	return runErr
}

func waitForDrain(ctx context.Context, srv *http.Server, drained <-chan error, forceBudget bool) error {
	if !forceBudget {
		select {
		case err := <-drained:
			return err
		case <-ctx.Done():
		}
	}
	budget, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	select {
	case err := <-drained:
		return err
	case <-budget.Done():
		err := errors.Join(budget.Err(), srv.Close())
		return errors.Join(err, <-drained)
	}
}

// Shutdown stops acceptance and waits for this run's drain, within ctx's budget.
// If the wait times out, a later call can keep waiting. Cancel the Run context to
// force connection closure after its cleanup budget instead.
func (e *Server) Shutdown(ctx context.Context) error {
	e.mutex.Lock()
	run := e.run
	e.mutex.Unlock()
	if run == nil {
		return nil
	}
	run.stopAccepting()
	select {
	case <-run.done:
		return run.err
	default:
	}
	select {
	case <-run.done:
		return run.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func listen(ctx context.Context, opts options) (net.Listener, *tls.Config, error) {
	var tlsConfig *tls.Config
	if opts.keyFile != "" || opts.certFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.certFile, opts.keyFile)
		if err != nil {
			return nil, nil, err
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", opts.addr)
	return l, tlsConfig, err
}

func serve(srv *http.Server, listener net.Listener) error {
	if srv.TLSConfig != nil {
		return srv.ServeTLS(listener, "", "")
	}
	return srv.Serve(listener)
}
