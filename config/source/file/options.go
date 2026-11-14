package file

import (
	"github.com/gatework/motor-core/config/source"
)

// WithPath sets the path to file
func WithPath(filePath string) source.Option {
	return func(o *source.Options) {
		o.FilePath = filePath
	}
}
