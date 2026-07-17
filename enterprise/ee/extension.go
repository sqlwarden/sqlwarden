// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise License. See enterprise/LICENSE.

// Package ee is the SQLWarden Enterprise root extension. In phase 1 it is a
// stub that exercises every extension seam end-to-end; real enterprise
// features replace its pieces in later phases.
package ee

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
	"github.com/sqlwarden/internal/response"
)

//go:embed migrations_postgres migrations_sqlite
var migrationFiles embed.FS

type Extension struct{}

func (Extension) Name() string { return "ee" }

func (Extension) Migrations(driver string) (fs.FS, bool) {
	dir := "migrations_postgres"
	if driver == "sqlite" {
		dir = "migrations_sqlite"
	}
	sub, err := fs.Sub(migrationFiles, dir)
	if err != nil {
		return nil, false
	}
	return sub, true
}

func (Extension) RegisterRoutes(r chi.Router, deps *extension.Deps) {
	r.Get("/api/v1/ee/stub", func(w http.ResponseWriter, req *http.Request) {
		err := response.JSON(w, http.StatusOK, map[string]any{
			"extension":     "ee",
			"edition":       deps.License.Edition(),
			"stub_licensed": deps.License.IsLicensed("stub"),
		})
		if err != nil {
			deps.Logger.ErrorContext(req.Context(), "ee stub response failed", "error", err)
		}
	})
}

func (Extension) Jobs(*extension.Deps) []jobs.Definition {
	return []jobs.Definition{{
		Type:        "ee_stub_noop",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
			return map[string]any{"ok": true}, nil
		}),
	}}
}

func (Extension) EventSink(deps *extension.Deps) events.Sink {
	return &debugLogSink{logger: deps.Logger}
}

func (Extension) LicenseService() license.Service { return placeholderLicense{} }

// debugLogSink proves event delivery into the enterprise edition. It logs
// only event shape fields, never payload data.
type debugLogSink struct {
	logger *slog.Logger
}

func (s *debugLogSink) HandleEvent(ctx context.Context, ev events.Event) {
	s.logger.DebugContext(ctx, "enterprise event sink received event",
		"event.action", ev.Action, "event.outcome", ev.Outcome)
}
