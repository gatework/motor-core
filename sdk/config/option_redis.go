package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisOptions uses URL when present; otherwise the individual address fields.
// The settings-file names remain compatible with existing Redis sections.
type RedisOptions struct {
	URL        string `yaml:"url" json:"url"`
	Network    string `yaml:"network" json:"network"`
	Addr       string `yaml:"addr" json:"addr"`
	Username   string `yaml:"username" json:"username"`
	Password   string `yaml:"password" json:"password"`
	DB         int    `yaml:"db" json:"db"`
	PoolSize   int    `yaml:"pool_size" json:"pool_size"`
	MaxRetries int    `yaml:"max_retries" json:"max_retries"`
	TLS        *TLS   `yaml:"tls" json:"tls"`
}

// TLS identifies optional client certificates and a trusted root bundle.
type TLS struct {
	Cert string `yaml:"cert" json:"cert"`
	Key  string `yaml:"key" json:"key"`
	CA   string `yaml:"ca" json:"ca"`
}

func (r RedisOptions) Options() (*goredis.Options, error) {
	if r.URL != "" {
		return goredis.ParseURL(r.URL)
	}
	if r.Addr == "" {
		return nil, errors.New("config: redis needs either url or addr")
	}
	opts := &goredis.Options{
		Network: r.Network, Addr: r.Addr, Username: r.Username, Password: r.Password,
		DB: r.DB, PoolSize: r.PoolSize, MaxRetries: r.MaxRetries,
	}
	var err error
	opts.TLSConfig, err = r.TLS.config()
	if err != nil {
		return nil, err
	}
	return opts, nil
}

const setupTimeout = 5 * time.Second

// Open creates a verified client owned by the caller; no process-global pool is
// retained. Callers can explicitly share it through storage's New constructors.
func (r RedisOptions) Open(ctx context.Context) (*goredis.Client, error) {
	opts, err := r.Options()
	if err != nil {
		return nil, err
	}
	opts.ContextTimeoutEnabled = true
	client := goredis.NewClient(opts)
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (t *TLS) config() (*tls.Config, error) {
	if t == nil {
		return nil, nil
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if t.Cert != "" || t.Key != "" {
		cert, err := tls.LoadX509KeyPair(t.Cert, t.Key)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}
	if t.CA == "" {
		return config, nil
	}
	data, err := os.ReadFile(t.CA)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("config: no certificate found in %q", t.CA)
	}
	config.RootCAs = pool
	return config, nil
}
