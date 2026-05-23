package web

import (
	"context"
	"net/http"
	"strconv"
)

func (app *application) personalSpacesEnabled(ctx context.Context) (bool, error) {
	settings, found, err := app.db.GetInstanceSettings(ctx)
	if err != nil {
		return false, err
	}
	if found {
		return settings.PersonalSpacesEnabled, nil
	}
	return app.config.PersonalSpacesEnabled, nil
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
