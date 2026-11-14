package bootstrap

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/config/source/memory"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
)

func newSource() source.Source {
	return memory.NewSource(memory.WithJSON([]byte(`{"settings":{"logger":{"level":"error"}}}`)))
}

func TestTheResourcePhaseIsAnnouncedAfterEveryCallback(t *testing.T) {
	app := runtime.New()
	var order []string
	app.SetPhase(runtime.AfterResource, func() {
		if config.Current().Settings.Application.Name != "committed" {
			t.Error("phase observed stale settings")
		}
		order = append(order, "phase")
	})
	watcher, err := SetupConfig(
		t.Context(),
		app,
		memory.NewSource(memory.WithJSON([]byte(`{"settings":{"application":{"name":"committed"},"logger":{"level":"error"}}}`))),
		func(*config.Settings) error { order = append(order, "first"); return nil },
		func(*config.Settings) error { order = append(order, "second"); return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if !reflect.DeepEqual(order, []string{"first", "second", "phase"}) {
		t.Fatalf("callback order = %v", order)
	}
}

func TestTheResourcePhaseIsAnnouncedWithNoCallbacks(t *testing.T) {
	app := runtime.New()
	ran := false
	app.SetPhase(runtime.AfterResource, func() { ran = true })
	watcher, err := SetupConfig(t.Context(), app, newSource())
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if !ran {
		t.Fatal("resource phase was not announced")
	}
}

func TestConfigSetupOnItsOwnAnnouncesNothing(t *testing.T) {
	app := runtime.New()
	ran := false
	app.SetPhase(runtime.AfterResource, func() { ran = true })
	watcher, err := config.Setup(t.Context(), newSource(), config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if ran {
		t.Fatal("config setup announced an unrelated application phase")
	}
}

func TestTheAnnouncementUsesTheSuppliedRuntime(t *testing.T) {
	app := runtime.New()
	other := runtime.New()
	ran := false
	other.SetPhase(runtime.AfterResource, func() { ran = true })
	watcher, err := SetupConfig(t.Context(), app, newSource())
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if ran {
		t.Fatal("phase was announced to another application")
	}
}

func TestTheCallersSliceIsNotWrittenTo(t *testing.T) {
	callbacks := make([]func(*config.Settings) error, 1, 4)
	callbacks[0] = func(*config.Settings) error { return nil }
	watcher, err := SetupConfig(t.Context(), runtime.New(), newSource(), callbacks...)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	for _, callback := range callbacks[1:cap(callbacks)] {
		if callback != nil {
			t.Fatal("caller callback slice was mutated")
		}
	}
}

func TestFailedSetupDoesNotAnnounceResources(t *testing.T) {
	app := runtime.New()
	ran := false
	app.SetPhase(runtime.AfterResource, func() { ran = true })
	failure := errors.New("resource setup failed")
	_, err := SetupConfig(t.Context(), app, newSource(), func(*config.Settings) error { return failure })
	if !errors.Is(err, failure) || ran {
		t.Fatalf("failed setup: error %v, announced %v", err, ran)
	}
}
