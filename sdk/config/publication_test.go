package config

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gatework/motor-core/config/loader"
	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/config/source/memory"
	"github.com/gatework/motor-core/logger"
)

func TestSetupRejectsMissingOrEmptySettings(t *testing.T) {
	isolateSettings(t)
	for name, data := range map[string]string{
		"null document":      "null",
		"empty document":     "",
		"missing settings":   `{}`,
		"unrelated document": `{"other":{"name":"ignored"}}`,
		"null settings":      `{"settings":null}`,
		"empty settings":     `{"settings":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			watch, err := Setup(t.Context(), memory.NewSource(memory.WithJSON([]byte(data))), Options{})
			if err == nil {
				_ = watch.Close()
				t.Fatal("incomplete settings were published")
			}
		})
	}
}

func TestRejectedEmptyReloadPreservesPublication(t *testing.T) {
	isolateSettings(t)
	w := &Watcher{}
	apply := func(data string) error {
		return w.apply(t.Context(), &loader.Snapshot{ChangeSet: &source.ChangeSet{Format: "json", Data: []byte(data)}})
	}
	if err := apply(`{"settings":{"application":{"name":"valid"}}}`); err != nil {
		t.Fatal(err)
	}
	previous := current.Load()
	for _, data := range []string{`null`, `{}`, `{"settings":null}`, `{"settings":{}}`} {
		if err := apply(data); err == nil {
			t.Errorf("accepted incomplete reload %s", data)
		}
		if current.Load() != previous {
			t.Errorf("incomplete reload %s replaced the last valid publication", data)
		}
	}
}

func TestWatcherBorrowsProcessLoggerWithoutOpeningOutputs(t *testing.T) {
	isolateSettings(t)
	var output bytes.Buffer
	root := logger.NewLogger(logger.WithOutput(&output))
	defer root.Close()
	logger.SetDefault(root)
	borrowed := root.With(logger.FieldString("scope", "request"))
	path := filepath.Join(t.TempDir(), "must-not-be-created")
	data, err := json.Marshal(map[string]any{"settings": map[string]any{
		"application": map[string]any{"name": "explicit-resources"},
		"logger":      map[string]any{"path": path, "level": "info"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := Setup(t.Context(), memory.NewSource(memory.WithJSON(data)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	if logger.Default() != root {
		t.Error("Setup replaced the process logger")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Setup opened a configured output: %v", err)
	}
	if err := watch.Close(); err != nil {
		t.Fatal(err)
	}
	borrowed.Log(context.Background(), logger.InfoLevel, "after watcher close")
	if !bytes.Contains(output.Bytes(), []byte("after watcher close")) {
		t.Fatal("closing the watcher closed a borrowed logger root")
	}
}

func TestSettingsValidateLoggerWithoutOpeningIt(t *testing.T) {
	for name, log := range map[string]Logger{
		"invalid adapter":   {Adapter: "invalid"},
		"invalid level":     {Level: "invalid"},
		"invalid encoder":   {Encoder: "invalid"},
		"negative rotation": {Rotation: &RotationConfig{MaxSize: -1}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := (&Settings{Settings: Config{Logger: log}}).validate(); err == nil {
				t.Fatal("invalid logger configuration accepted")
			}
		})
	}
}
