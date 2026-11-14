package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Server coordinates registered services and their shutdown budget.
type Server struct {
	mu        sync.Mutex
	services  map[string]Runnable
	isRunning bool
	opts      options
}

// New constructs a service manager with a shared shutdown budget.
func New(opts ...Option) *Server {
	s := &Server{services: make(map[string]Runnable), opts: setDefaultOptions()}
	for _, o := range opts {
		o(&s.opts)
	}
	return s
}

// Add registers a named service before a run begins.
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

// Run joins registered services and coordinates their graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	names, services, timeout, err := s.begin()
	if err != nil {
		return err
	}
	var runs, shutdowns sync.WaitGroup
	allCompleted := false
	release := func() {
		runs.Wait()
		shutdowns.Wait()
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
	}
	defer func() {
		if allCompleted {
			release()
			return
		}
		// A timeout bounds the wait, not user code. Keep the run reserved until
		// its remaining tasks have actually returned.
		go release()
	}()
	results := make(chan serviceResult, len(services)*2)
	cancels := make([]context.CancelFunc, len(services))
	for i, r := range services {
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		cancels[i] = cancel
		runs.Go(func() { results <- serviceResult{name: names[i], err: r.Run(runCtx)} })
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
		allCompleted = true
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
		shutdowns.Go(func() {
			defer cancels[i]()
			results <- serviceResult{name: names[i], err: closer.Shutdown(shutdownCtx)}
		})
	}
	result, err = collectShutdown(shutdownCtx, results, pending, result)
	allCompleted = err == nil
	return result
}

func collectShutdown(ctx context.Context, results <-chan serviceResult, pending int, result error) (error, error) {
	for pending > 0 {
		select {
		case r := <-results:
			pending--
			if r.err != nil && !errors.Is(r.err, context.Canceled) {
				result = errors.Join(result, fmt.Errorf("server: %s: %w", r.name, r.err))
			}
		case <-ctx.Done():
			return errors.Join(result, fmt.Errorf("server: shutdown: %w", ctx.Err())), ctx.Err()
		}
	}
	return result, nil
}

func (s *Server) begin() ([]string, []Runnable, time.Duration, error) {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return nil, nil, 0, errors.New("server: already running")
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
	return names, services, timeout, nil
}
