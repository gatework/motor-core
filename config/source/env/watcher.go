package env

import (
	"github.com/gatework/motor-core/config/source"
	"sync"
)

type watcher struct {
	exit chan struct{}
	once sync.Once
}

func (w *watcher) Next() (*source.ChangeSet, error) {
	<-w.exit

	return nil, source.ErrWatcherStopped
}

func (w *watcher) Stop() error {
	w.once.Do(func() { close(w.exit) })
	return nil
}

func newWatcher() (source.Watcher, error) {
	return &watcher{exit: make(chan struct{})}, nil
}
