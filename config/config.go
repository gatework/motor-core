// Package config loads and watches dynamic configuration.
package config

import (
	"github.com/gatework/motor-core/config/loader"
	"github.com/gatework/motor-core/config/reader"
	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/config/source/file"
)

// Watcher is the config watcher
type Watcher interface {
	Next() (reader.Value, error)
	Stop() error
}

// Entity 配置实体
type Entity interface {
	OnChange()
}

// Options 配置的参数
type Options struct {
	Loader loader.Loader
	Reader reader.Reader
	Source []source.Source

	Entity Entity
}

// Option 调用类型
type Option func(o *Options)

// New returns an initialized configuration manager. Call Close to release its
// owned loader and watchers; a loader supplied through WithLoader stays borrowed.
func New(opts ...Option) (*Config, error) { return newConfig(opts...) }

// LoadFile adds a file source to this configuration manager.
func (c *Config) LoadFile(path string) error {
	return c.Load(file.NewSource(file.WithPath(path)))
}
