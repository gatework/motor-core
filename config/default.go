package config

import (
	"bytes"
	stdjson "encoding/json"
	"errors"
	"sync"
	"time"

	"log/slog"

	"github.com/gatework/motor-core/config/loader"
	"github.com/gatework/motor-core/config/loader/memory"
	"github.com/gatework/motor-core/config/reader"
	"github.com/gatework/motor-core/config/reader/json"
	"github.com/gatework/motor-core/config/source"
)

// Config owns the current configuration snapshot and its reload watcher.
type Config struct {
	exit chan bool
	done chan struct{}
	// exitOnce guards exit: the select-then-close in Close is a check followed
	// by an act, so two callers can both reach the close.
	exitOnce   sync.Once
	ownsLoader bool
	closeErr   error
	opts       Options

	updateMu sync.Mutex
	mu       sync.RWMutex
	// the current snapshot
	snap *loader.Snapshot
	// the current values
	vals reader.Values
}

type watcher struct {
	lw    loader.Watcher
	rd    reader.Reader
	path  []string
	value reader.Value
}

func newConfig(opts ...Option) (*Config, error) {
	var c Config

	err := c.init(opts...)
	if err != nil {
		return nil, err
	}
	c.done = make(chan struct{})
	go c.run()

	return &c, nil
}

func (c *Config) init(opts ...Option) (initErr error) {
	defer func() {
		if initErr != nil && c.ownsLoader {
			_ = c.opts.Loader.Close()
		}
	}()
	c.opts = Options{
		Reader: json.NewReader(),
	}
	c.exit = make(chan bool)
	for _, o := range opts {
		o(&c.opts)
	}

	// default loader uses the configured reader
	c.ownsLoader = c.opts.Loader == nil
	if c.ownsLoader {
		c.opts.Loader = memory.NewLoader(memory.WithReader(c.opts.Reader))
	}

	err := c.opts.Loader.Load(c.opts.Source...)
	if err != nil {
		return err
	}

	c.snap, err = c.opts.Loader.Snapshot()
	if err != nil {
		return err
	}

	c.vals, err = c.opts.Reader.Values(c.snap.ChangeSet)
	if err != nil {
		return err
	}
	if c.opts.Entity != nil {
		if err := c.vals.Scan(c.opts.Entity); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) Options() Options {
	options := c.opts
	options.Source = append([]source.Source(nil), options.Source...)
	return options
}

func (c *Config) run() {
	defer close(c.done)
	watch := func(w loader.Watcher) error {
		for {
			// get changeset 获取变更集
			snap, err := w.Next()
			if err != nil {
				return err
			}

			if err := c.apply(snap, true); err != nil {
				slog.Error("config: apply snapshot", "error", err)
			}

		}
	}

	for {
		w, err := c.opts.Loader.Watch()
		if err != nil {
			slog.Error("config: start watcher", "error", err)
			// The exit check at the bottom sits past the point this jumps
			// back from, so without racing it here a loader that cannot be
			// watched keeps this goroutine retrying after Close.
			select {
			case <-c.exit:
				return
			case <-time.After(time.Second):
			}
			continue
		}

		done := make(chan bool)

		// the stop watch func 停止监控函数
		go func() {
			select {
			case <-done:
			case <-c.exit:
			}
			_ = w.Stop()
		}()

		// block watch 阻塞监控
		if err := watch(w); err != nil {
			// 记录错误日志
			slog.Error("config: watch", "error", err)
			select {
			case <-c.exit:
				close(done)
				return
			case <-time.After(time.Second):
			}
		}

		// Closed per iteration rather than deferred: a defer inside this loop
		// only runs when run returns, so every iteration left its stop-watch
		// goroutine alive until then.
		close(done)

		// if the config is closed exit 检查是否退出
		select {
		case <-c.exit:
			return
		default:
		}
	}
}

func (c *Config) apply(snap *loader.Snapshot, notify bool) error {
	c.updateMu.Lock()
	c.mu.RLock()
	stale := c.snap != nil && c.snap.Version >= snap.Version
	c.mu.RUnlock()
	if stale {
		c.updateMu.Unlock()
		return nil
	}
	vals, err := c.opts.Reader.Values(snap.ChangeSet)
	if err != nil {
		c.updateMu.Unlock()
		return err
	}
	if c.opts.Entity != nil {
		if err := vals.Scan(c.opts.Entity); err != nil {
			c.updateMu.Unlock()
			return err
		}
	}
	c.mu.Lock()
	c.snap = snap
	c.vals = vals
	c.mu.Unlock()
	c.updateMu.Unlock()
	if notify && c.opts.Entity != nil {
		c.opts.Entity.OnChange()
	}
	return nil
}

func (c *Config) Map() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vals.Map()
}

func (c *Config) Scan(v any) error {
	// User-defined UnmarshalJSON methods run without holding manager locks.
	return c.Get().Scan(v)
}

// Sync sync loads all the sources, calls the parser and updates the config
func (c *Config) Sync() error {
	if err := c.opts.Loader.Sync(); err != nil {
		return err
	}

	snap, err := c.opts.Loader.Snapshot()
	if err != nil {
		return err
	}

	return c.apply(snap, false)
}

func (c *Config) Close() error {
	c.exitOnce.Do(func() {
		close(c.exit)
		if c.ownsLoader {
			c.closeErr = c.opts.Loader.Close()
		}
	})
	if c.done != nil {
		<-c.done
	}
	return c.closeErr
}

func (c *Config) Get(path ...string) reader.Value {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// did sync actually work?
	if c.vals != nil {
		return c.vals.Get(path...)
	}

	// no value
	return newValue()
}

// Set encodes caller values before taking the configuration lock. A failed
// conversion leaves the current snapshot unchanged.
func (c *Config) Set(val any, path ...string) error {
	data, err := stdjson.Marshal(val)
	if err != nil {
		return err
	}
	var normalized any
	decoder := stdjson.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vals == nil {
		return errors.New("config: values are not initialized")
	}
	return c.vals.Set(normalized, path...)
}

func (c *Config) Del(path ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.vals != nil {
		c.vals.Del(path...)
	}

}

func (c *Config) Bytes() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.vals == nil {
		return []byte{}
	}

	return c.vals.Bytes()
}

func (c *Config) Load(sources ...source.Source) error {
	if err := c.opts.Loader.Load(sources...); err != nil {
		return err
	}

	snap, err := c.opts.Loader.Snapshot()
	if err != nil {
		return err
	}

	return c.apply(snap, false)
}

func (c *Config) Watch(path ...string) (Watcher, error) {
	readerValue := c.Get(path...)

	w, err := c.opts.Loader.Watch(path...)
	if err != nil {
		return nil, err
	}

	return &watcher{
		lw:    w,
		rd:    c.opts.Reader,
		path:  path,
		value: readerValue,
	}, nil
}

func (c *Config) String() string {
	return "config"
}

func (w *watcher) Next() (reader.Value, error) {
	for {
		s, err := w.lw.Next()
		if err != nil {
			return nil, err
		}

		// only process changes
		if bytes.Equal(w.value.Bytes(), s.ChangeSet.Data) {
			continue
		}

		v, err := w.rd.Values(s.ChangeSet)
		if err != nil {
			return nil, err
		}

		w.value = v.Get()
		return w.value, nil
	}
}

func (w *watcher) Stop() error {
	return w.lw.Stop()
}
