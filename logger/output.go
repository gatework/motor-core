package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// openOutput creates only the file resources requested by this logger. Writers
// provided through WithOutput remain caller-owned.
func openOutput(opts Options) (io.Writer, io.Closer, error) {
	writers := make([]io.Writer, 0, 3)
	if opts.Out != nil {
		writers = append(writers, opts.Out)
	}
	if opts.Stdout && opts.Out == nil {
		writers = append(writers, os.Stdout)
	}
	var owned io.Closer
	if opts.Path != "" {
		path := opts.Path
		if filepath.Ext(path) == "" {
			path = filepath.Join(path, "app.log")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, fmt.Errorf("logger: create log directory: %w", err)
		}
		// Lumberjack opens lazily. Check the destination now so construction
		// cannot succeed with a file that will reject the first log entry.
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("logger: open log file: %w", err)
		}
		if err := file.Close(); err != nil {
			return nil, nil, fmt.Errorf("logger: close log probe: %w", err)
		}
		rotation := &lumberjack.Logger{
			Filename: path, MaxSize: opts.MaxSize, MaxBackups: opts.MaxBackups,
			MaxAge: opts.MaxAge, Compress: opts.Compress, LocalTime: opts.LocalTime,
		}
		owned = rotation
		writers = append(writers, rotation)
	}
	if len(writers) == 0 {
		return os.Stderr, nil, nil
	}
	if len(writers) == 1 {
		return writers[0], owned, nil
	}
	return io.MultiWriter(writers...), owned, nil
}
