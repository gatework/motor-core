package memory

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/gatework/motor-core/config/loader"
	"github.com/gatework/motor-core/config/reader"
	"github.com/gatework/motor-core/config/reader/json"
	"github.com/gatework/motor-core/config/source"
)

type memory struct {
	exit    chan struct{}
	once    sync.Once
	workers sync.WaitGroup
	opts    loader.Options

	// Operations serialize source reads and merges without blocking readers.
	operation sync.Mutex
	sync.RWMutex
	snap     *loader.Snapshot
	vals     reader.Values
	sets     []*source.ChangeSet
	sources  []source.Source
	watchers map[*watcher]struct{}
	version  uint64
}

type updateValue struct {
	version string
	value   reader.Value
}

type watcher struct {
	exit     chan struct{}
	exitOnce sync.Once
	path     []string
	value    reader.Value
	reader   reader.Reader
	version  string
	updates  chan struct{}
	mu       sync.Mutex
	pending  updateValue
	remove   func()
}

func (m *memory) watch(idx int, s source.Source, initial source.Watcher) {
	defer m.workers.Done()
	w := initial
	for {
		select {
		case <-m.exit:
			if w != nil {
				_ = w.Stop()
			}
			return
		default:
		}
		if w == nil {
			var err error
			w, err = s.Watch()
			if err != nil {
				if !m.retry() {
					return
				}
				continue
			}
			// Catch writes made while a failed watcher was being restarted.
			if cs, err := s.Read(); err == nil {
				_ = m.replace(idx, cs)
			}
		}

		finished := make(chan struct{})
		stopped := make(chan struct{})
		go func(active source.Watcher) {
			defer close(stopped)
			select {
			case <-m.exit:
			case <-finished:
			}
			_ = active.Stop()
		}(w)

		for {
			cs, err := w.Next()
			if err != nil {
				break
			}
			// A malformed update leaves the previous snapshot usable and does
			// not tear down a watcher that can deliver a later correction.
			_ = m.replace(idx, cs)
		}
		close(finished)
		<-stopped
		w = nil
		if !m.retry() {
			return
		}
	}
}

func (m *memory) retry() bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-m.exit:
		return false
	case <-timer.C:
		return true
	}
}

func copySet(cs *source.ChangeSet) *source.ChangeSet {
	if cs == nil {
		return nil
	}
	result := *cs
	result.Data = bytes.Clone(cs.Data)
	return &result
}

func (m *memory) replace(idx int, cs *source.ChangeSet) error {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.RLock()
	if m.snap == nil {
		m.RUnlock()
		return nil
	}
	sets := slices.Clone(m.sets)
	m.RUnlock()
	sets[idx] = copySet(cs)
	return m.publish(sets)
}

// publish is called with operation held. Failed decoding never replaces state.
func (m *memory) publish(sets []*source.ChangeSet) error {
	set, err := m.opts.Reader.Merge(sets...)
	if err != nil {
		return err
	}
	vals, err := m.opts.Reader.Values(set)
	if err != nil {
		return err
	}
	m.Lock()
	defer m.Unlock()
	select {
	case <-m.exit:
		return source.ErrWatcherStopped
	default:
	}
	m.version++
	m.sets = sets
	m.vals = vals
	m.snap = &loader.Snapshot{ChangeSet: set, Version: fmt.Sprintf("%020d", m.version)}
	m.updateLocked()
	return nil
}

func (m *memory) update() {
	m.RLock()
	defer m.RUnlock()
	m.updateLocked()
}

func (m *memory) updateLocked() {
	if m.snap == nil {
		return
	}
	for w := range m.watchers {
		w.offer(updateValue{version: m.snap.Version, value: m.vals.Get(w.path...)})
	}
}

func (w *watcher) offer(update updateValue) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if update.version <= w.pending.version {
		return
	}
	w.pending = update
	select {
	case w.updates <- struct{}{}:
	default:
	}
}

func (m *memory) Snapshot() (*loader.Snapshot, error) {
	m.RLock()
	if m.snap != nil {
		snap := loader.Copy(m.snap)
		m.RUnlock()
		return snap, nil
	}
	m.RUnlock()
	if err := m.Sync(); err != nil {
		return nil, err
	}
	m.RLock()
	defer m.RUnlock()
	return loader.Copy(m.snap), nil
}

func (m *memory) Sync() error {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.RLock()
	sources := slices.Clone(m.sources)
	m.RUnlock()
	sets := make([]*source.ChangeSet, len(sources))
	for i, s := range sources {
		cs, err := s.Read()
		if err != nil {
			return fmt.Errorf("read source %s: %w", s.String(), err)
		}
		sets[i] = copySet(cs)
	}
	return m.publish(sets)
}

func (m *memory) Close() error {
	m.once.Do(func() {
		m.Lock()
		close(m.exit)
		watchers := make([]*watcher, 0, len(m.watchers))
		for w := range m.watchers {
			watchers = append(watchers, w)
		}
		m.Unlock()
		for _, w := range watchers {
			_ = w.Stop()
		}
	})
	m.workers.Wait()
	return nil
}

func (m *memory) Get(path ...string) (reader.Value, error) {
	if _, err := m.Snapshot(); err != nil {
		return nil, err
	}
	m.RLock()
	defer m.RUnlock()
	return m.vals.Get(path...), nil
}

func (m *memory) Load(sources ...source.Source) error {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.RLock()
	sets := slices.Clone(m.sets)
	m.RUnlock()
	watches := make([]source.Watcher, len(sources))
	for i, s := range sources {
		if s == nil {
			for _, w := range watches {
				if w != nil {
					_ = w.Stop()
				}
			}
			return errors.New("config: nil source")
		}
		// Register before Read; a write between those calls is then either in
		// the initial snapshot or waiting for the watcher, never lost.
		watches[i], _ = s.Watch()
		cs, err := s.Read()
		if err != nil {
			for _, w := range watches {
				if w != nil {
					_ = w.Stop()
				}
			}
			return fmt.Errorf("read source %s: %w", s.String(), err)
		}
		sets = append(sets, copySet(cs))
	}
	if err := m.publish(sets); err != nil {
		for _, w := range watches {
			if w != nil {
				_ = w.Stop()
			}
		}
		return err
	}
	m.Lock()
	select {
	case <-m.exit:
		m.Unlock()
		for _, w := range watches {
			if w != nil {
				_ = w.Stop()
			}
		}
		return source.ErrWatcherStopped
	default:
	}
	start := len(m.sources)
	m.sources = append(m.sources, sources...)
	m.workers.Add(len(sources))
	m.Unlock()
	for i, s := range sources {
		go m.watch(start+i, s, watches[i])
	}
	return nil
}

func (m *memory) Watch(path ...string) (loader.Watcher, error) {
	if _, err := m.Snapshot(); err != nil {
		return nil, err
	}
	m.Lock()
	defer m.Unlock()
	select {
	case <-m.exit:
		return nil, source.ErrWatcherStopped
	default:
	}
	w := &watcher{
		exit: make(chan struct{}), path: slices.Clone(path), reader: m.opts.Reader,
		updates: make(chan struct{}, 1),
	}
	w.remove = func() {
		m.Lock()
		delete(m.watchers, w)
		m.Unlock()
	}
	m.watchers[w] = struct{}{}
	w.offer(updateValue{version: m.snap.Version, value: m.vals.Get(path...)})
	return w, nil
}

func (m *memory) String() string { return "memory" }

func (w *watcher) Next() (*loader.Snapshot, error) {
	for {
		select {
		case <-w.exit:
			return nil, source.ErrWatcherStopped
		case <-w.updates:
		}
		w.mu.Lock()
		uv := w.pending
		w.mu.Unlock()
		if uv.version <= w.version {
			continue
		}
		w.version = uv.version
		if w.value != nil && bytes.Equal(w.value.Bytes(), uv.value.Bytes()) {
			continue
		}
		w.value = uv.value
		cs := &source.ChangeSet{
			Data: uv.value.Bytes(), Format: w.reader.String(), Source: "memory", Timestamp: time.Now(),
		}
		cs.Checksum = cs.Sum()
		return &loader.Snapshot{ChangeSet: cs, Version: uv.version}, nil
	}
}

func (w *watcher) Stop() error {
	w.exitOnce.Do(func() {
		close(w.exit)
		if w.remove != nil {
			w.remove()
		}
	})
	return nil
}

func NewLoader(opts ...loader.Option) loader.Loader {
	options := loader.Options{Reader: json.NewReader()}
	for _, o := range opts {
		o(&options)
	}
	m := &memory{
		exit: make(chan struct{}), opts: options, watchers: make(map[*watcher]struct{}),
		sources: slices.Clone(options.Source), sets: make([]*source.ChangeSet, len(options.Source)),
	}
	for i, s := range options.Source {
		w, _ := s.Watch()
		m.workers.Add(1)
		go m.watch(i, s, w)
	}
	return m
}
