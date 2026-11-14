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

// New 实例化
func New(name string, opts ...Option) *Server {
	s := &Server{
		name: name,
		opts: setDefaultOption(),
	}
	s.Options(opts...)
	return s
}

// NewMetrics 新建默认监控服务
func NewMetrics(opts ...Option) *Server {
	s := &Server{
		name: "metrics",
		opts: setDefaultOption(),
	}
	s.opts.addr = ":3000"
	h := http.NewServeMux()
	h.Handle("/metrics", promhttp.Handler())
	s.opts.handler = h
	s.Options(opts...)
	return s
}

// NewHealthz 默认健康检查服务
func NewHealthz(opts ...Option) *Server {
	s := &Server{
		name: "healthz",
		opts: setDefaultOption(),
	}
	s.opts.addr = ":4000"
	h := http.NewServeMux()
	h.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s.opts.handler = h
	s.Options(opts...)
	return s
}

func NewReadyz(opts ...Option) *Server {
	s := &Server{
		name: "readyz",
		opts: setDefaultOption(),
	}
	s.opts.addr = ":2000"
	h := http.NewServeMux()
	h.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s.opts.handler = h
	s.Options(opts...)
	return s
}

// Options 设置参数
func (e *Server) Options(opts ...Option) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	for _, o := range opts {
		o(&e.opts)
	}
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

	var tlsConfig *tls.Config
	if opts.keyFile != "" || opts.certFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.certFile, opts.keyFile)
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	lc := net.ListenConfig{}
	l, err := lc.Listen(startCtx, "tcp", opts.addr)
	if err != nil {
		select {
		case <-run.stop:
			return nil
		default:
			return err
		}
	}
	defer l.Close()
	srv := &http.Server{Handler: opts.handler, BaseContext: func(net.Listener) context.Context { return ctx }}
	if tlsConfig != nil {
		l = tls.NewListener(l, tlsConfig)
	}
	serving := make(chan error, 1)
	go func() { serving <- srv.Serve(l) }()
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
