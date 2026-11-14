package memory

import (
	"github.com/gatework/motor-core/config/source"
)

func withData(d []byte, f string) source.Option {
	return func(o *source.Options) {
		o.ChangeSet = &source.ChangeSet{Data: d, Format: f}
	}
}

// WithChangeSet allows a changeset to be set
func WithChangeSet(cs *source.ChangeSet) source.Option {
	return func(o *source.Options) {
		o.ChangeSet = cs
	}
}

// WithJSON allows the source data to be set to json
func WithJSON(d []byte) source.Option {
	return withData(d, "json")
}

// WithYAML allows the source data to be set to yaml
func WithYAML(d []byte) source.Option {
	return withData(d, "yaml")
}
