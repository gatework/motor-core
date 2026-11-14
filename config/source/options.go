package source

import (
	"github.com/gatework/motor-core/config/encoder"
	"github.com/gatework/motor-core/config/encoder/json"
)

// Options contains typed construction settings shared by configuration sources.
type Options struct {
	// Encoder
	Encoder encoder.Encoder

	// Source-specific settings are explicit rather than carried in context values.
	FilePath         string
	ChangeSet        *ChangeSet
	Prefixes         []string
	StrippedPrefixes []string
	IncludeUnset     bool
}

// Option changes configuration source settings.
type Option func(o *Options)

// NewOptions applies source options to the JSON encoder default.
func NewOptions(opts ...Option) Options {
	options := Options{
		Encoder: json.NewEncoder(),
	}

	for _, o := range opts {
		o(&options)
	}

	return options
}

// WithEncoder sets the source encoder
func WithEncoder(e encoder.Encoder) Option {
	return func(o *Options) {
		o.Encoder = e
	}
}
