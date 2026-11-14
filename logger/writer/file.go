package writer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const timeFormat = "2006-01-02"

// File synchronously writes and rotates log files. Close releases its
// file descriptor. Use logger.NewAsyncLogger when asynchronous logging is needed.
type File struct {
	mutex sync.Mutex
	file  *os.File
	// FilenameFunc is configured before Write and must not call back into the writer.
	FilenameFunc func(*File) string
	num          uint
	opts         Options
}

var _ io.WriteCloser = (*File)(nil)

// New opens the current log file with the supplied rotation options.
func New(opts ...Option) (*File, error) {
	w := &File{opts: setDefault()}
	for _, opt := range opts {
		opt(&w.opts)
	}
	filename := w.getFilename()
	if w.opts.cap > 0 {
		for {
			_, err := os.Stat(filename)
			if errors.Is(err, os.ErrNotExist) {
				if w.num > 0 {
					w.num--
					filename = w.getFilename()
				}
				break
			}
			if err != nil {
				return nil, err
			}
			w.num++
			filename = w.getFilename()
		}
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0o600)
	if err != nil {
		return nil, err
	}
	w.file = file
	return w, nil
}

// Write consumes data before returning and reports file or rotation failures.
func (w *File) Write(data []byte) (int, error) {
	if w == nil {
		return 0, errors.New("writer: nil file writer")
	}
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if err := w.rotate(); err != nil {
		return 0, err
	}
	return w.file.Write(data)
}

// Close releases the file. It is safe to call more than once.
func (w *File) Close() error {
	if w == nil {
		return nil
	}
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *File) rotate() error {
	info, err := w.file.Stat()
	if err != nil {
		return err
	}
	isToday := strings.Contains(w.file.Name(), time.Now().Format(timeFormat))
	isFull := w.opts.cap > 0 && uint64(info.Size()) > uint64(w.opts.cap)
	if isToday && !isFull {
		return nil
	}
	previous := w.num
	if isToday {
		w.num++
	} else {
		w.num = 0
	}
	next, err := os.OpenFile(w.getFilename(), os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0o600)
	if err != nil {
		w.num = previous
		return err
	}
	old := w.file
	w.file = next
	return old.Close()
}

func (w *File) getFilename() string {
	if w.FilenameFunc != nil {
		return w.FilenameFunc(w)
	}
	name := time.Now().Format(timeFormat)
	if w.opts.cap > 0 {
		name = fmt.Sprintf("%s-[%d]", name, w.num)
	}
	return filepath.Join(w.opts.path, name+"."+w.opts.suffix)
}
