package env

import (
	"strings"

	"github.com/gatework/motor-core/config/source"
)

// WithStrippedPrefix sets the environment variable prefixes to scope to.
// These prefixes will be removed from the actual config entries.
func WithStrippedPrefix(p ...string) source.Option {
	return func(o *source.Options) {
		o.StrippedPrefixes = appendUnderscore(p)
	}
}

// WithPrefix sets the environment variable prefixes to scope to.
// These prefixes will not be removed. Each prefix will be considered a top level config entry.
func WithPrefix(p ...string) source.Option {
	return func(o *source.Options) {
		o.Prefixes = appendUnderscore(p)
	}
}

func appendUnderscore(prefixes []string) []string {
	result := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if !strings.HasSuffix(p, "_") {
			result = append(result, p+"_")
			continue
		}

		result = append(result, p)
	}

	return result
}
