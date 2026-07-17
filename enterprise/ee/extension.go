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
		if err := deps.License.Require("stub"); err != nil {
			deps.Logger.InfoContext(req.Context(), "ee route denied without license", "route", "/api/v1/ee/stub")
			_ = extension.WriteLicenseRequired(w, "stub")
			return
		}
		err := response.JSON(w, http.StatusOK, map[string]any{
			"extension": "ee",
			"edition":   deps.License.Edition(),
		})
		if err != nil {
			deps.Logger.ErrorContext(req.Context(), "ee stub response failed", "error", err)
		}
	})
}

func (Extension) Jobs(deps *extension.Deps) []jobs.Definition {
	return []jobs.Definition{{
		Type:        "ee_stub_noop",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
			// Jobs re-check the license at execution time: a job enqueued
			// while licensed must not run after the license lapses.
			if err := deps.License.Require("stub"); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		}),
	}}
}

func (Extension) EventSink(deps *extension.Deps) events.Sink {
	return &debugLogSink{logger: deps.Logger, license: deps.License}
}

func (Extension) LicenseService() license.Service { return placeholderLicense{} }

// debugLogSink proves event delivery into the enterprise edition. It logs
// only event shape fields, never payload data. Sinks check the license per
// event so an unlicensed binary processes nothing and a key applied at
// runtime takes effect without restart.
type debugLogSink struct {
	logger  *slog.Logger
	license license.Service
}

func (s *debugLogSink) HandleEvent(ctx context.Context, ev events.Event) {
	if !s.license.IsLicensed("stub") {
		return
	}
	s.logger.DebugContext(ctx, "enterprise event sink received event",
		"event.action", ev.Action, "event.outcome", ev.Outcome)
}
