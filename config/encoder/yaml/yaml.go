// Package yaml encodes configuration values as YAML.
package yaml

import (
	"github.com/ghodss/yaml"

	"github.com/gatework/motor-core/config/encoder"
)

type yamlEncoder struct{}

func (y yamlEncoder) Encode(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

func (y yamlEncoder) Decode(d []byte, v any) error {
	return yaml.Unmarshal(d, v)
}

func (y yamlEncoder) String() string {
	return "yaml"
}

// NewEncoder constructs a YAML configuration encoder.
func NewEncoder() encoder.Encoder {
	return yamlEncoder{}
}
