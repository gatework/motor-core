package memory

import (
	"bytes"
	"sync"

	"github.com/gatework/motor-core/config/source"
)

type watcher struct {
	id       string
	updates  chan *source.ChangeSet
	source   *memory
	done     chan struct{}
	stopOnce sync.Once
}

func (w *watcher) Next() (*source.ChangeSet, error) {
	select {
	case <-w.done:
		return nil, source.ErrWatcherStopped
	default:
	}
	select {
	case <-w.done:
		return nil, source.ErrWatcherStopped
	case cs := <-w.updates:
		cloned := *cs
		cloned.Data = bytes.Clone(cs.Data)
		return &cloned, nil
	}
}

func (w *watcher) Stop() error {
	w.stopOnce.Do(func() {
		w.source.Lock()
		delete(w.source.Watchers, w.id)
		close(w.done)
		w.source.Unlock()
	})
	return nil
}
