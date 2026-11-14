// Package file is a file source. Expected format is json
package file

import (
	"io"
	"os"

	"github.com/gatework/motor-core/config/source"
)

type file struct {
	path string
	opts source.Options
}

var (
	// DefaultPath is the configuration file used when no path is supplied.
	DefaultPath = "config.json"
)

func (f *file) Read() (*source.ChangeSet, error) {
	fh, err := os.Open(f.path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	b, err := io.ReadAll(fh)
	if err != nil {
		return nil, err
	}
	info, err := fh.Stat()
	if err != nil {
		return nil, err
	}

	cs := &source.ChangeSet{
		Format:    format(f.path, f.opts.Encoder),
		Source:    f.String(),
		Timestamp: info.ModTime(),
		Data:      b,
	}
	cs.Checksum = cs.Sum()

	return cs, nil
}

func (f *file) String() string {
	return "file"
}

func (f *file) Watch() (source.Watcher, error) {
	if _, err := os.Stat(f.path); err != nil {
		return nil, err
	}
	return newWatcher(f)
}

func (f *file) Write(cs *source.ChangeSet) error {
	return nil
}

// NewSource constructs a file-backed configuration source.
func NewSource(opts ...source.Option) source.Source {
	options := source.NewOptions(opts...)
	path := DefaultPath
	if options.FilePath != "" {
		path = options.FilePath
	}
	return &file{opts: options, path: path}
}
