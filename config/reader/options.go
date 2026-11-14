package reader

import (
	"github.com/gatework/motor-core/config/encoder"
	"github.com/gatework/motor-core/config/encoder/json"
	"github.com/gatework/motor-core/config/encoder/toml"
	"github.com/gatework/motor-core/config/encoder/xml"
	"github.com/gatework/motor-core/config/encoder/yaml"
)

// Options selects the encoders available to a reader.
type Options struct {
	Encoding map[string]encoder.Encoder
}

// Option changes reader construction settings.
type Option func(o *Options)

// NewOptions applies options to the built-in configuration encoders.
func NewOptions(opts ...Option) Options {
	options := Options{
		Encoding: map[string]encoder.Encoder{
			"json": json.NewEncoder(),
			"yaml": yaml.NewEncoder(),
			"toml": toml.NewEncoder(),
			"xml":  xml.NewEncoder(),
			"yml":  yaml.NewEncoder(),
		},
	}
	for _, o := range opts {
		o(&options)
	}
	return options
}

// WithEncoder registers an encoder under its format name.
func WithEncoder(e encoder.Encoder) Option {
	return func(o *Options) {
		if o.Encoding == nil {
			o.Encoding = make(map[string]encoder.Encoder)
		}
		o.Encoding[e.String()] = e
	}
}
