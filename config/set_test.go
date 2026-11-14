package config

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/config/source/memory"
)

type reentrantJSONValue struct {
	config  *Config
	blocked atomic.Bool
}

func (v *reentrantJSONValue) MarshalJSON() ([]byte, error) {
	done := make(chan struct{})
	go func() {
		_ = v.config.Get("initial").String("")
		close(done)
	}()
	select {
	case <-done:
		return []byte(`"updated"`), nil
	case <-time.After(2 * time.Second):
		v.blocked.Store(true)
		return nil, errors.New("configuration locked during user marshaling")
	}
}

func TestSetMarshalsOutsideConfigurationLock(t *testing.T) {
	config, err := New(WithSource(memory.NewSource(memory.WithJSON([]byte(`{"initial":"value"}`)))))
	if err != nil {
		t.Fatal(err)
	}
	defer config.Close()
	value := &reentrantJSONValue{config: config}
	if err := config.Set(value, "result"); err != nil {
		t.Fatal(err)
	}
	if value.blocked.Load() {
		t.Fatal("Set held the configuration lock while invoking MarshalJSON")
	}
	if got := config.Get("result").String(""); got != "updated" {
		t.Fatalf("Set produced %q, want updated", got)
	}
}

func TestSetReportsEncodingErrorsWithoutChangingValues(t *testing.T) {
	config, err := New(WithSource(memory.NewSource(memory.WithJSON([]byte(`{"value":"original"}`)))))
	if err != nil {
		t.Fatal(err)
	}
	defer config.Close()
	if err := config.Set(make(chan int), "value"); err == nil {
		t.Fatal("Set swallowed the encoding error")
	}
	if got := config.Get("value").String(""); got != "original" {
		t.Fatalf("failed Set changed value to %q", got)
	}
	const large = uint64(9007199254740993)
	if err := config.Set(map[string]any{"number": large}, "object"); err != nil {
		t.Fatal(err)
	}
	var got struct{ Number uint64 }
	if err := config.Get("object").Scan(&got); err != nil || got.Number != large {
		t.Fatalf("Set lost numeric precision: %d, %v", got.Number, err)
	}
}
