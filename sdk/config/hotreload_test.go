package config

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/config/source/file"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/runtime"
	"github.com/gatework/motor-core/storage"
)

type labelledCache struct {
	storage.Cache
	label string
}
type labelledQueue struct {
	storage.Queue
	label string
}

const settingsTemplate = `settings:
  application:
    host: 0.0.0.0
    port: 8000
    name: %s
    mode: dev
  logger:
    level: error
  jwt:
    secret: test
    timeout: 3600
  database:
    driver: sqlite3
    source: ":memory:"
`

func isolateSettings(t *testing.T) {
	t.Helper()
	previous := current.Load()
	previousLog := logger.Default()
	t.Cleanup(func() { current.Store(previous); logger.SetDefault(previousLog) })
}

func TestConfigReloadReplacesResourcesAfterTheRegistriesAreSealed(t *testing.T) {
	isolateSettings(t)
	path := filepath.Join(t.TempDir(), "settings.yml")
	writeSettings(t, path, "v1")
	app := runtime.New()
	app.RunBefore()
	app.RunAppRouters()
	setup := func(settings *Settings) error {
		label := settings.Settings.Application.Name
		app.SetCache(&labelledCache{label: label})
		app.SetQueue(&labelledQueue{label: label})
		app.SetDB(label, &gorm.DB{})
		return nil
	}
	watcher, err := Setup(t.Context(), file.NewSource(file.WithPath(path)), Options{Callbacks: []func(*Settings) error{setup}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = watcher.Close() })
	if got := app.Cache().(*labelledCache).label; got != "v1" {
		t.Fatalf("initial cache label = %q", got)
	}
	first := Current()
	first.Settings.Application.Name = "caller mutation"
	first.Settings.Databases["*"] = Database{Driver: "changed"}
	if Current().Settings.Application.Name != "v1" || Current().Settings.Databases["*"].Driver != "sqlite3" {
		t.Fatal("Current leaked a mutable settings snapshot")
	}
	writeSettings(t, path, "v2")
	waitFor(t, 5*time.Second, func() bool { return Current().Settings.Application.Name == "v2" }, "settings did not reload")
	if app.Cache().(*labelledCache).label != "v2" || app.Queue().(*labelledQueue).label != "v2" || app.DB("v2") == nil {
		t.Fatal("reload failed to replace resources")
	}
	if !app.BeforeSealed() || !app.AppRoutersSealed() {
		t.Fatal("reload reopened a sealed registry")
	}

	rejected := make(reloadErrorWriter, 1)
	previousLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(rejected, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLog) })
	if err := os.WriteFile(path, []byte("settings: {application: {name: rejected, port: -1}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-rejected:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher never reported the rejected reload")
	}

	if got := Current().Settings.Application.Name; got != "v2" {
		t.Fatalf("invalid reload published %q", got)
	}
}

type reloadErrorWriter chan struct{}

func (w reloadErrorWriter) Write(data []byte) (int, error) {
	if bytes.Contains(data, []byte("rejected reload")) {
		select {
		case w <- struct{}{}:
		default:
		}
	}
	return len(data), nil
}

func TestSetupRejectsInvalidInitialSettings(t *testing.T) {
	isolateSettings(t)
	path := filepath.Join(t.TempDir(), "settings.yml")
	if err := os.WriteFile(path, []byte("settings: {application: {port: invalid}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := current.Load()
	if _, err := Setup(context.Background(), file.NewSource(file.WithPath(path)), Options{}); err == nil {
		t.Fatal("invalid initial scan succeeded")
	}
	if current.Load() != previous {
		t.Fatal("failed setup replaced Current")
	}
}

func writeSettings(t *testing.T, path, name string) {
	t.Helper()
	if err := os.WriteFile(path+".tmp", []byte(fmt.Sprintf(settingsTemplate, name)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, limit time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}
