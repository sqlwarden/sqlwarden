package web

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/platform/observability"
	"github.com/sqlwarden/internal/settings"
)

// InitialSettings returns the instance settings NewApplication applied. A
// settings watcher seeds from them so it applies only later changes.
func (app *application) InitialSettings() settings.Instance {
	return app.initialSettings
}

// SettingsAppliers returns the appliers that put instance settings into
// effect for this web application: the dependency appliers first, so SMTP
// settings that cannot be built reject the change before anything else is
// applied, then web's own.
func (app *application) SettingsAppliers() []settings.Applier {
	appliers := append([]settings.Applier(nil), app.settingsAppliers...)
	return append(appliers, app.applyLogLevel, app.applyAccessLogs)
}

// ApplyInstanceSettings runs every settings applier in order and stops at the
// first error.
func (app *application) ApplyInstanceSettings(ctx context.Context, in settings.Instance) error {
	for _, apply := range app.SettingsAppliers() {
		if err := apply(ctx, in); err != nil {
			return err
		}
	}
	return nil
}

func (app *application) applyLogLevel(_ context.Context, in settings.Instance) error {
	if err := observability.SetLevel(app.logger, in.LogLevel); err != nil {
		return fmt.Errorf("apply runtime log level: %w", err)
	}
	return nil
}

func (app *application) applyAccessLogs(_ context.Context, in settings.Instance) error {
	app.accessLogsEnabled.Store(in.AccessLogsEnabled)
	return nil
}
