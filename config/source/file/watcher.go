package file

import (
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/gatework/motor-core/config/source"
)

type watcher struct {
	f    *file
	fw   *fsnotify.Watcher
	exit chan struct{}
	once sync.Once
	err  error
}

func newWatcher(f *file) (source.Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// Watch the directory so replacing the file atomically does not detach us.
	if err := fw.Add(filepath.Dir(f.path)); err != nil {
		_ = fw.Close()
		return nil, err
	}
	return &watcher{f: f, fw: fw, exit: make(chan struct{})}, nil
}

func (w *watcher) Next() (*source.ChangeSet, error) {
	for {
		select {
		case <-w.exit:
			return nil, source.ErrWatcherStopped
		case event, ok := <-w.fw.Events:
			if !ok {
				return nil, source.ErrWatcherStopped
			}
			if filepath.Clean(event.Name) != filepath.Clean(w.f.path) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			return w.f.Read()
		case err, ok := <-w.fw.Errors:
			if !ok {
				return nil, source.ErrWatcherStopped
			}
			return nil, err
		}
	}
}

func (w *watcher) Stop() error {
	w.once.Do(func() { close(w.exit); w.err = w.fw.Close() })
	return w.err
}
