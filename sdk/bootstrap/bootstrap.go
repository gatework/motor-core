// Package bootstrap wires settings to an explicitly supplied application.
package bootstrap

import (
	"context"
	"errors"

	"github.com/gatework/motor-core/config/source"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
)

// SetupConfig applies resource callbacks before announcing AfterResource, on
// both startup and reload. The caller owns the returned watcher and closes it
// before shutting down the resources the callbacks installed.
func SetupConfig(
	ctx context.Context,
	app *runtime.Application,
	s source.Source,
	callbacks ...func(*config.Settings) error,
) (*config.Watcher, error) {
	if app == nil {
		return nil, errors.New("bootstrap: nil application")
	}
	return config.Setup(ctx, s, config.Options{
		Callbacks: callbacks,
		OnApplied: func(*config.Settings) { app.RunPhase(runtime.AfterResource) },
	})
}
