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
	name      string
	mutex     sync.Mutex
	srv       *http.Server
	opts      options
	isStarted bool
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

// Run binds before reporting readiness and returns the serving failure, if any.
func (e *Server) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	e.mutex.Lock()
	if e.isStarted {
		e.mutex.Unlock()
		return errors.New("listener: already running")
	}
	e.isStarted = true
	opts := e.opts
	e.mutex.Unlock()
	defer func() { e.mutex.Lock(); e.isStarted = false; e.srv = nil; e.mutex.Unlock() }()
	var tlsConfig *tls.Config
	if opts.keyFile != "" || opts.certFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.certFile, opts.keyFile)
		if err != nil {
			return err
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}
	lc := net.ListenConfig{}
	l, err := lc.Listen(ctx, "tcp", opts.addr)
	if err != nil {
		return err
	}
	defer l.Close()
	srv := &http.Server{Handler: opts.handler, BaseContext: func(net.Listener) context.Context { return ctx }}
	e.mutex.Lock()
	e.srv = srv
	e.mutex.Unlock()
	if tlsConfig != nil {
		l = tls.NewListener(l, tlsConfig)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	if opts.startedHook != nil {
		opts.startedHook()
	}
	select {
	case err = <-done:
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			_ = srv.Close()
			err = shutdownErr
		}
		serveErr := <-done
		if err == nil {
			err = serveErr
		}
	}
	if opts.endHook != nil {
		opts.endHook()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops acceptance and waits for active HTTP requests.
func (e *Server) Shutdown(ctx context.Context) error {
	e.mutex.Lock()
	srv := e.srv
	e.mutex.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
