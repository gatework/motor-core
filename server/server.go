package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Server coordinates registered services and their shutdown budget.
type Server struct {
	mu        sync.Mutex
	services  map[string]Runnable
	isRunning bool
	opts      options
}

func New(opts ...Option) *Server {
	s := &Server{services: make(map[string]Runnable), opts: setDefaultOptions()}
	for _, o := range opts {
		o(&s.opts)
	}
	return s
}

func (s *Server) Add(name string, r Runnable) error {
	if name == "" || r == nil {
		return errors.New("server: name and runnable are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isRunning {
		return errors.New("server: cannot register while running")
	}
	if s.services == nil {
		s.services = make(map[string]Runnable)
	}
	s.services[name] = r
	return nil
}

type serviceResult struct {
	name string
	err  error
}

func (s *Server) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return errors.New("server: already running")
	}
	s.isRunning = true
	names := make([]string, 0, len(s.services))
	for name := range s.services {
		names = append(names, name)
	}
	sort.Strings(names)
	services := make([]Runnable, len(names))
	for i, name := range names {
		services[i] = s.services[name]
	}
	timeout := s.opts.gracefulShutdownTimeout
	s.mu.Unlock()
	var runs, shutdowns sync.WaitGroup
	defer func() {
		// A timeout ends the caller's wait, not arbitrary user code. Do not allow
		// another run until every previously owned task has actually returned.
		go func() { runs.Wait(); shutdowns.Wait(); s.mu.Lock(); s.isRunning = false; s.mu.Unlock() }()
	}()
	results := make(chan serviceResult, len(services)*2)
	cancels := make([]context.CancelFunc, len(services))
	for i, r := range services {
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		cancels[i] = cancel
		runs.Add(1)
		go func() { defer runs.Done(); results <- serviceResult{name: names[i], err: r.Run(runCtx)} }()
	}
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
	}()
	pending := len(services)
	var result error
running:
	for pending > 0 {
		select {
		case <-ctx.Done():
			break running
		case r := <-results:
			pending--
			if r.err != nil {
				result = fmt.Errorf("server: %s: %w", r.name, r.err)
				break running
			}
		}
	}
	if pending == 0 {
		return result
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	for i, r := range services {
		closer, ok := r.(shutdowner)
		if !ok {
			cancels[i]()
			continue
		}
		pending++
		shutdowns.Add(1)
		go func() {
			defer shutdowns.Done()
			defer cancels[i]()
			results <- serviceResult{name: names[i], err: closer.Shutdown(shutdownCtx)}
		}()
	}
	for pending > 0 {
		select {
		case r := <-results:
			pending--
			if r.err != nil && !errors.Is(r.err, context.Canceled) {
				result = errors.Join(result, fmt.Errorf("server: %s: %w", r.name, r.err))
			}
		case <-shutdownCtx.Done():
			return errors.Join(result, fmt.Errorf("server: shutdown: %w", shutdownCtx.Err()))
		}
	}
	return result
}
