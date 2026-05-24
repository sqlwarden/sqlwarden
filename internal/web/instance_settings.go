package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/database"
)

func (app *application) personalSpacesEnabled(ctx context.Context) (bool, error) {
	settings, err := app.instanceSettings(ctx)
	if err != nil {
		return false, err
	}
	return settings.PersonalSpacesEnabled, nil
}

func (app *application) instanceSettings(ctx context.Context) (database.InstanceSettings, error) {
	settings, found, err := app.db.GetInstanceSettings(ctx)
	if err != nil {
		return database.InstanceSettings{}, err
	}
	if !found {
		return app.defaultInstanceSettings(), nil
	}
	if settings.InstanceName == "" {
		settings.InstanceName = app.defaultInstanceSettings().InstanceName
	}
	if settings.PublicURL == "" {
		settings.PublicURL = app.config.BaseURL
	}
	return settings, nil
}

func (app *application) defaultInstanceSettings() database.InstanceSettings {
	return database.InstanceSettings{
		InstanceName:          "SQLWarden",
		PublicURL:             app.config.BaseURL,
		SupportEmail:          app.config.Notifications.Email,
		PersonalSpacesEnabled: app.config.PersonalSpacesEnabled,
	}
}

func (app *application) instanceSettingsResponse(settings database.InstanceSettings) map[string]any {
	return map[string]any{
		"instance_name":             settings.InstanceName,
		"instance_description":      settings.InstanceDescription,
		"support_email":             settings.SupportEmail,
		"public_url":                settings.PublicURL,
		"personal_spaces_enabled":   settings.PersonalSpacesEnabled,
		"deployment_mode":           app.config.DeploymentMode,
		"access_mode":               app.config.AccessMode,
		"single_user_mode":          app.config.AccessMode == AccessModeSingleUser,
		"personal_spaces_default":   app.config.PersonalSpacesEnabled,
		"settings_source":           "database",
		"runtime_settings_readonly": true,
	}
}

func (app *application) dropPersonalSpaceSessions(ctx context.Context) error {
	connIDs, err := app.db.ListPersonalConnectionIDs(ctx)
	if err != nil {
		return err
	}
	for _, connID := range connIDs {
		app.connManager.RemoveForConnection(strconv.FormatInt(connID, 10))
	}
	return nil
}

func (app *application) requirePersonalSpacesEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled, err := app.personalSpacesEnabled(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !enabled {
			app.notFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
