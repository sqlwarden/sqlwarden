// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

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
	"github.com/sqlwarden/internal/response"
)

//go:embed migrations_postgres migrations_sqlite
var migrationFiles embed.FS

func NewModule() extension.Module {
	return extension.Module{
		Name:              "ee",
		Migrations:        migrations,
		CapabilityFactory: newCapabilityGate,
		Start:             start,
	}
}

func migrations(driver string) (fs.FS, bool) {
	dir := "migrations_postgres"
	switch driver {
	case "postgres":
	case "sqlite":
		dir = "migrations_sqlite"
	default:
		return nil, false
	}
	sub, err := fs.Sub(migrationFiles, dir)
	if err != nil {
		return nil, false
	}
	return sub, true
}

func start(_ context.Context, deps extension.RuntimeDeps) (extension.Contributions, error) {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		err := response.JSON(w, http.StatusOK, map[string]any{
			"extension": "ee",
		})
		if err != nil {
			deps.Logger.ErrorContext(req.Context(), "ee stub response failed", "error", err)
		}
	})

	return extension.Contributions{
		Routes: []extension.Route{{
			Scope:      extension.RouteInstanceAdmin,
			Prefix:     "/ee/stub",
			Capability: "stub",
			Handler:    r,
		}},
		Jobs: []extension.Job{{
			Capability: "stub",
			Definition: jobs.Definition{
				Type:        "ee_stub_noop",
				MaxAttempts: 1,
				Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
					return map[string]any{"ok": true}, nil
				}),
			},
		}},
		EventSinks: []extension.EventSink{{
			Capability: "stub",
			Sink:       &debugLogSink{logger: deps.Logger},
		}},
	}, nil
}

// debugLogSink proves event delivery into the enterprise edition. It logs
// only event shape fields, never payload data. Core checks capability state
// per event so runtime availability changes take effect without restart.
type debugLogSink struct {
	logger *slog.Logger
}

func (s *debugLogSink) HandleEvent(ctx context.Context, ev events.Event) error {
	s.logger.DebugContext(ctx, "enterprise event sink received event",
		"event.action", ev.Action, "event.outcome", ev.Outcome)
	return nil
}
