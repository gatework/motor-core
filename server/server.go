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
		runs.Go(func() { ; results <- serviceResult{name: names[i], err: r.Run(runCtx)} })
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
	allCompleted = true
	return result
}
