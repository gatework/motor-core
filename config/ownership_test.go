package config

import (
	"sync"
	"testing"
	"time"

	"github.com/gatework/motor-core/config/loader"
	loadermemory "github.com/gatework/motor-core/config/loader/memory"
	"github.com/gatework/motor-core/config/source"
	sourcememory "github.com/gatework/motor-core/config/source/memory"
)

type ownedSource struct {
	source.Source
	started   chan struct{}
	stopped   chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}
type ownedWatcher struct {
	source.Watcher
	source *ownedSource
}

func (s *ownedSource) Watch() (source.Watcher, error) {
	w, err := s.Source.Watch()
	if err != nil {
		return nil, err
	}
	s.startOnce.Do(func() { close(s.started) })
	return &ownedWatcher{Watcher: w, source: s}, nil
}
func (w *ownedWatcher) Stop() error {
	err := w.Watcher.Stop()
	w.source.stopOnce.Do(func() { close(w.source.stopped) })
	return err
}
func TestCloseStopsOwnedSourceWatcher(t *testing.T) {
	s := &ownedSource{Source: sourcememory.NewSource(sourcememory.WithJSON([]byte(`{}`))), started: make(chan struct{}), stopped: make(chan struct{})}
	c, err := New(WithSource(s))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case <-s.started:
	case <-time.After(time.Second):
		t.Fatal("source watcher did not start")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.stopped:
	case <-time.After(time.Second):
		t.Fatal("owned source watcher did not stop")
	}
}

type borrowedLoader struct {
	loader.Loader
	closed bool
}

func (l *borrowedLoader) Close() error { l.closed = true; return l.Loader.Close() }
func TestCloseDoesNotCloseBorrowedLoader(t *testing.T) {
	l := &borrowedLoader{Loader: loadermemory.NewLoader()}
	defer l.Loader.Close()
	c, err := New(WithLoader(l))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if l.closed {
		t.Fatal("closed a caller-owned loader")
	}
}
