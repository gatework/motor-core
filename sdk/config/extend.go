package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"sync"
)

type extendSlot interface {
	decode(json.RawMessage) (any, error)
}

type typedExtendSlot[T any] struct{}

func (*typedExtendSlot[T]) decode(data json.RawMessage) (any, error) {
	value := new(T)
	if err := json.Unmarshal(data, value); err != nil {
		return nil, err
	}
	return value, nil
}

var (
	extendMu      sync.RWMutex
	extendTargets = map[string]extendSlot{}
)

// RegisterExtend claims a typed extension section before Setup runs. Its
// accessor returns a read-only snapshot; reloads never mutate a returned value.
// Missing sections retain their previous successful value. Duplicate or empty
// keys are programming errors and panic at registration.
func RegisterExtend[T any](key string) func() *T {
	if key == "" {
		panic("config: RegisterExtend called with an empty key")
	}
	extendMu.Lock()
	if _, duplicate := extendTargets[key]; duplicate {
		extendMu.Unlock()
		panic("config: RegisterExtend called twice for key " + fmt.Sprintf("%q", key))
	}
	extendTargets[key] = &typedExtendSlot[T]{}
	extendMu.Unlock()
	zero := new(T)
	return func() *T {
		if snapshot := current.Load(); snapshot != nil {
			if value, ok := snapshot.extensions[key].(*T); ok {
				return value
			}
		}
		return zero
	}
}

// All sections are decoded before one atomic publication with built-in settings.
func prepareExtensions(raw map[string]json.RawMessage) (map[string]any, error) {
	extendMu.RLock()
	slots := make(map[string]extendSlot, len(extendTargets))
	maps.Copy(slots, extendTargets)
	extendMu.RUnlock()
	values := make(map[string]any, len(slots))
	if previous := current.Load(); previous != nil {
		maps.Copy(values, previous.extensions)
	}
	for key, slot := range slots {
		data, ok := raw[key]
		if !ok {
			continue
		}
		value, err := slot.decode(data)
		if err != nil {
			return nil, fmt.Errorf("config: extend section %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}
