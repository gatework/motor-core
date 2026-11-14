package flag

import (
	"github.com/gatework/motor-core/config/source"
)

// IncludeUnset toggles the loading of unset flags and their respective default values.
// Default behavior is to ignore any unset flags.
func IncludeUnset(b bool) source.Option {
	return func(o *source.Options) {
		o.IncludeUnset = b
	}
}
