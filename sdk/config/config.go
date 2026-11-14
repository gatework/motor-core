package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/gatework/motor-core/config/loader"
	"github.com/gatework/motor-core/config/loader/memory"
	jsonreader "github.com/gatework/motor-core/config/reader/json"
	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/logger"
)

// Settings is a complete settings-file snapshot.
type Settings struct {
	Settings Config `yaml:"settings" json:"settings"`
}

// Config contains the built-in sections. Names in settings files are unchanged.
type Config struct {
	Application Application                `yaml:"application" json:"application"`
	SSL         SSL                        `yaml:"ssl" json:"ssl"`
	Logger      Logger                     `yaml:"logger" json:"logger"`
	JWT         JWT                        `yaml:"jwt" json:"jwt"`
	Database    Database                   `yaml:"database" json:"database"`
	Databases   map[string]Database        `yaml:"databases" json:"databases"`
	Gen         Gen                        `yaml:"gen" json:"gen"`
	Cache       Cache                      `yaml:"cache" json:"cache"`
	Queue       Queue                      `yaml:"queue" json:"queue"`
	Extend      map[string]json.RawMessage `yaml:"extend" json:"extend"`
}

type publishedSettings struct {
	settings   *Settings
	extensions map[string]any
}

var current atomic.Pointer[publishedSettings]

// Current returns a copy of the last successfully loaded settings. The caller
// owns the result, including maps, slices, and optional configuration sections.
func Current() *Settings {
	snapshot := current.Load()
	if snapshot == nil {
		return &Settings{Settings: Config{Databases: map[string]Database{}, Extend: map[string]json.RawMessage{}}}
	}
	return cloneSettings(snapshot.settings)
}

func cloneSettings(settings *Settings) *Settings {
	copy := *settings
	copy.Settings.Database = cloneDatabase(settings.Settings.Database)
	copy.Settings.Databases = make(map[string]Database, len(settings.Settings.Databases))
	for name, db := range settings.Settings.Databases {
		copy.Settings.Databases[name] = cloneDatabase(db)
	}
	copy.Settings.Extend = make(map[string]json.RawMessage, len(settings.Settings.Extend))
	for name, data := range settings.Settings.Extend {
		copy.Settings.Extend[name] = bytes.Clone(data)
	}
	if rotation := settings.Settings.Logger.Rotation; rotation != nil {
		value := *rotation
		copy.Settings.Logger.Rotation = &value
	}
	copy.Settings.Cache.Redis = cloneRedis(settings.Settings.Cache.Redis)
	if settings.Settings.Cache.Memory != nil {
		copy.Settings.Cache.Memory = &CacheMemory{}
	}
	if redis := settings.Settings.Queue.Redis; redis != nil {
		value := *redis
		value.RedisOptions = *cloneRedis(&redis.RedisOptions)
		copy.Settings.Queue.Redis = &value
	}
	if memory := settings.Settings.Queue.Memory; memory != nil {
		value := *memory
		copy.Settings.Queue.Memory = &value
	}
	return &copy
}

func cloneDatabase(db Database) Database {
	db.Registers = slices.Clone(db.Registers)
	for i := range db.Registers {
		db.Registers[i].Sources = slices.Clone(db.Registers[i].Sources)
		db.Registers[i].Replicas = slices.Clone(db.Registers[i].Replicas)
		db.Registers[i].Tables = slices.Clone(db.Registers[i].Tables)
	}
	return db
}

func cloneRedis(redis *RedisOptions) *RedisOptions {
	if redis == nil {
		return nil
	}
	copy := *redis
	if redis.TLS != nil {
		tls := *redis.TLS
		copy.TLS = &tls
	}
	return &copy
}

func (c *Config) multiDatabase() {
	if len(c.Databases) == 0 {
		c.Databases = map[string]Database{"*": c.Database}
		return
	}
	if _, wildcard := c.Databases["*"]; wildcard && len(c.Databases) > 1 {
		others := make([]string, 0, len(c.Databases)-1)
		for name := range c.Databases {
			if name != "*" {
				others = append(others, name)
			}
		}
		sort.Strings(others)
		slog.Warn("config: wildcard database hides per-tenant entries", "ignored", others)
	}
}

func (s *Settings) validate() error {
	app := s.Settings.Application
	if app.Port < 0 || app.Port > 65535 {
		return errors.New("config: application port must be between 0 and 65535")
	}
	if app.ReadTimeout < 0 || app.WriterTimeout < 0 {
		return errors.New("config: application timeouts cannot be negative")
	}
	if s.Settings.JWT.Timeout < 0 || s.Settings.JWT.MaxRefresh < 0 {
		return errors.New("config: jwt durations cannot be negative")
	}
	for _, redis := range []*RedisOptions{s.Settings.Cache.Redis, queueRedis(s.Settings.Queue.Redis)} {
		if redis != nil {
			if _, err := redis.Options(); err != nil {
				return err
			}
		}
	}
	return nil
}

func queueRedis(q *RedisQueue) *RedisOptions {
	if q == nil {
		return nil
	}
	return &q.RedisOptions
}

// Watcher owns its configuration loader and reload goroutine. Close stops source
// watchers and waits for any callback in progress. Call Close outside callbacks.
type Watcher struct {
	loader    loader.Loader
	watch     loader.Watcher
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
	closeErr  error
	callbacks []func(*Settings) error
	onApplied func(*Settings)
	log       logger.Logger
}

// Options defines preparation callbacks and the notification after publication.
type Options struct {
	Callbacks []func(*Settings) error
	// OnApplied observes committed settings on startup and every successful
	// reload. It cannot reject a commit and runs outside configuration locks.
	OnApplied func(*Settings)
}

// Setup validates and publishes the initial settings, then starts watching.
// Callbacks receive candidate snapshots before publication; a rejected candidate
// leaves Current and registered extension values unchanged. Callbacks must clean
// up resources they created if they return an error.
func Setup(ctx context.Context, s source.Source, options Options) (*Watcher, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("config: nil source")
	}
	l := memory.NewLoader()
	if err := l.Load(s); err != nil {
		_ = l.Close()
		return nil, err
	}
	watch, err := l.Watch()
	if err != nil {
		_ = l.Close()
		return nil, err
	}
	snapshot, err := watch.Next()
	if err != nil {
		_ = l.Close()
		return nil, err
	}
	watchCtx, cancel := context.WithCancel(ctx)
	w := &Watcher{loader: l, watch: watch, cancel: cancel, done: make(chan struct{}), callbacks: slices.Clone(options.Callbacks), onApplied: options.OnApplied}
	if err := w.apply(watchCtx, snapshot); err != nil {
		cancel()
		_ = l.Close()
		return nil, err
	}
	go w.run(watchCtx)
	return w, nil
}

func (w *Watcher) apply(ctx context.Context, snapshot *loader.Snapshot) error {
	values, err := jsonreader.NewReader().Values(snapshot.ChangeSet)
	if err != nil {
		return err
	}
	next := &Settings{}
	if err := values.Scan(next); err != nil {
		return err
	}
	if err := next.validate(); err != nil {
		return err
	}
	next.Settings.multiDatabase()
	extensions, err := prepareExtensions(next.Settings.Extend)
	if err != nil {
		return err
	}
	log, err := next.Settings.Logger.Open()
	if err != nil {
		return err
	}
	for _, callback := range w.callbacks {
		if callback == nil {
			continue
		}
		if err := callback(cloneSettings(next)); err != nil {
			if closer, ok := log.(io.Closer); ok {
				_ = closer.Close()
			}
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		if closer, ok := log.(io.Closer); ok {
			_ = closer.Close()
		}
		return err
	}
	current.Store(&publishedSettings{settings: next, extensions: extensions})
	previous := w.log
	w.log = log
	logger.SetDefault(log)
	if closer, ok := previous.(io.Closer); ok {
		_ = closer.Close()
	}
	if w.onApplied != nil {
		w.onApplied(cloneSettings(next))
	}
	return nil
}

func (w *Watcher) run(ctx context.Context) {
	defer close(w.done)
	defer w.loader.Close()
	defer func() {
		if closer, ok := w.log.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = w.watch.Stop() })
	defer stop()
	for {
		snapshot, err := w.watch.Next()
		if err != nil {
			return
		}
		if err := w.apply(ctx, snapshot); err != nil && ctx.Err() == nil {
			slog.Error("config: rejected reload", "error", err)
		}
	}
}

func (w *Watcher) Close() error {
	w.once.Do(func() {
		w.cancel()
		_ = w.watch.Stop()
		w.closeErr = w.loader.Close()
	})
	<-w.done
	return w.closeErr
}
