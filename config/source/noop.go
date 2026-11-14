package source

import (
	"sync"
)

// NoopWatcher blocks until stopped and never reports configuration changes.
// Its zero value behaves as an already stopped watcher.
type NoopWatcher struct {
	exit chan struct{}
	once sync.Once
}

// NewNoopWatcher returns a watcher that blocks on Next() until Stop() is called.
func NewNoopWatcher() (*NoopWatcher, error) {
	return &NoopWatcher{exit: make(chan struct{})}, nil
}

// Next blocks until Stop and then returns ErrWatcherStopped.
func (w *NoopWatcher) Next() (*ChangeSet, error) {
	if w.exit == nil {
		return nil, ErrWatcherStopped
	}
	<-w.exit

	return nil, ErrWatcherStopped
}

// Stop unblocks all waiting Next calls. It is safe to call repeatedly.
func (w *NoopWatcher) Stop() error {
	if w.exit != nil {
		w.once.Do(func() { close(w.exit) })
	}
	return nil
}
