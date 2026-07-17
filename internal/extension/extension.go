// Package extension defines the seam through which edition-specific modules
// plug into the core application. Core wiring iterates a Registry and checks
// which optional capability interfaces each Extension implements; community
// builds run with an empty registry. Extensions must not reach into core
// internals beyond what Deps exposes.
package extension

import (
	"io/fs"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
)

type Extension interface {
	Name() string
}

type Deps struct {
	DB      *database.DB
	Logger  *slog.Logger
	License license.Service
	Events  *events.Bus
}

// MigrationSource supplies an extension's own migration stream. The
// filesystem must be rooted at a directory of golang-migrate .sql files for
// the given app-database driver ("postgres" or "sqlite"). Versions are
// tracked in a per-extension table, never in core's schema_migrations.
type MigrationSource interface {
	Migrations(driver string) (fs.FS, bool)
}

// RouteRegistrar mounts extension routes on the root router. Extensions own
// their full middleware stack for the paths they claim.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router, deps *Deps)
}

type JobProvider interface {
	Jobs(deps *Deps) []jobs.Definition
}

type EventSinkProvider interface {
	EventSink(deps *Deps) events.Sink
}

// LicenseProvider replaces the community license service. At most one
// registered extension should implement it; the last one wins.
type LicenseProvider interface {
	LicenseService() license.Service
}

type Registry struct {
	extensions []Extension
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Add(exts ...Extension) {
	r.extensions = append(r.extensions, exts...)
}

func (r *Registry) All() []Extension {
	return append([]Extension(nil), r.extensions...)
}
