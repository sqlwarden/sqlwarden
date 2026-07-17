// Package extension defines the explicit composition seam through which
// edition-specific modules contribute behavior to the core application.
package extension

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
)

var validModuleName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// RouteScope identifies the core security boundary applied before an
// extension handler is reached.
type RouteScope string

const (
	RoutePublic        RouteScope = "public"
	RouteAccount       RouteScope = "account"
	RouteInstanceAdmin RouteScope = "instance_admin"
	RouteOrganization  RouteScope = "organization"
)

// Route is mounted beneath the API root for its Scope. Prefix must be an
// absolute path relative to that scope. Feature is always enforced by core.
// Organization routes must also declare the required organization permission.
type Route struct {
	Scope      RouteScope
	Prefix     string
	Feature    string
	Permission string
	Handler    http.Handler
}

// Job is a centrally license-gated job definition.
type Job struct {
	Feature    string
	Definition jobs.Definition
}

// EventSink is a centrally license-gated best-effort event consumer.
type EventSink struct {
	Feature string
	Sink    events.Sink
}

// Contributions are created after core runtime dependencies are ready.
// Closers are invoked in reverse order during application shutdown.
type Contributions struct {
	Routes     []Route
	Jobs       []Job
	EventSinks []EventSink
	Closers    []io.Closer
}

// BootstrapDeps are available while constructing the active license service.
// LookupEnv and Now make environment-backed license configuration and expiry
// checks deterministic in tests without coupling enterprise code to web.Config.
type BootstrapDeps struct {
	DB        *database.DB
	Logger    *slog.Logger
	LookupEnv func(string) (string, bool)
	Now       func() time.Time
}

// RuntimeDeps are available when a module creates its runtime contributions.
type RuntimeDeps struct {
	DB      *database.DB
	Logger  *slog.Logger
	License license.Service
	Events  *events.Bus
}

type MigrationSource func(driver string) (fs.FS, bool)
type LicenseFactory func(context.Context, BootstrapDeps) (license.Service, error)
type StartFunc func(context.Context, RuntimeDeps) (Contributions, error)

// Module is an explicit edition module manifest. Optional functions may be
// nil, but names are unique and all returned contributions are validated.
type Module struct {
	Name           string
	Migrations     MigrationSource
	LicenseFactory LicenseFactory
	Start          StartFunc
}

// Registry is immutable after application startup. Validation turns all
// accidental collisions and malformed manifests into startup errors.
type Registry struct {
	modules []Module
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Add(modules ...Module) {
	r.modules = append(r.modules, modules...)
}

func (r *Registry) Modules() []Module {
	if r == nil {
		return nil
	}
	return append([]Module(nil), r.modules...)
}

func (r *Registry) Validate() error {
	if r == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(r.modules))
	licenseProviders := 0
	for _, module := range r.modules {
		if !validModuleName.MatchString(module.Name) {
			return fmt.Errorf("extension module name %q must match %s", module.Name, validModuleName)
		}
		if _, exists := seen[module.Name]; exists {
			return fmt.Errorf("extension module name %q is registered more than once", module.Name)
		}
		seen[module.Name] = struct{}{}
		if module.LicenseFactory != nil {
			licenseProviders++
		}
	}
	if licenseProviders > 1 {
		return fmt.Errorf("extension registry has %d license providers; exactly one is allowed", licenseProviders)
	}
	return nil
}

// LicenseService constructs the registry's license service after the database
// is initialized. Community registries use the deny-all community service.
func (r *Registry) LicenseService(ctx context.Context, deps BootstrapDeps) (license.Service, error) {
	if deps.LookupEnv == nil {
		deps.LookupEnv = os.LookupEnv
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	for _, module := range r.Modules() {
		if module.LicenseFactory == nil {
			continue
		}
		svc, err := module.LicenseFactory(ctx, deps)
		if err != nil {
			return nil, fmt.Errorf("extension %s license service: %w", module.Name, err)
		}
		if svc == nil {
			return nil, fmt.Errorf("extension %s returned a nil license service", module.Name)
		}
		return svc, nil
	}
	return license.Community(), nil
}

// Start initializes modules in registration order and validates every runtime
// contribution before any background worker or HTTP listener can use it.
func (r *Registry) Start(ctx context.Context, deps RuntimeDeps) (Contributions, error) {
	var all Contributions
	for _, module := range r.Modules() {
		if module.Start == nil {
			continue
		}
		contrib, err := module.Start(ctx, deps)
		if err != nil {
			closeContributions(all.Closers)
			return Contributions{}, fmt.Errorf("start extension %s: %w", module.Name, err)
		}
		if err := validateContributions(module.Name, contrib); err != nil {
			closeContributions(contrib.Closers)
			closeContributions(all.Closers)
			return Contributions{}, err
		}
		all.Routes = append(all.Routes, contrib.Routes...)
		all.Jobs = append(all.Jobs, contrib.Jobs...)
		all.EventSinks = append(all.EventSinks, contrib.EventSinks...)
		all.Closers = append(all.Closers, contrib.Closers...)
	}
	if err := validateAggregateContributions(all); err != nil {
		closeContributions(all.Closers)
		return Contributions{}, err
	}
	return all, nil
}

func validateAggregateContributions(contrib Contributions) error {
	routes := make(map[string]struct{}, len(contrib.Routes))
	for _, route := range contrib.Routes {
		key := string(route.Scope) + ":" + route.Prefix
		if _, exists := routes[key]; exists {
			return fmt.Errorf("extension route %s is registered more than once", key)
		}
		routes[key] = struct{}{}
	}
	jobTypes := make(map[string]struct{}, len(contrib.Jobs))
	for _, job := range contrib.Jobs {
		if _, exists := jobTypes[job.Definition.Type]; exists {
			return fmt.Errorf("extension job type %q is registered more than once", job.Definition.Type)
		}
		jobTypes[job.Definition.Type] = struct{}{}
	}
	return nil
}

func validateContributions(module string, contrib Contributions) error {
	for i, route := range contrib.Routes {
		if route.Handler == nil {
			return fmt.Errorf("extension %s route %d has no handler", module, i)
		}
		if !strings.HasPrefix(route.Prefix, "/") || route.Prefix == "/" {
			return fmt.Errorf("extension %s route %d has invalid prefix %q", module, i, route.Prefix)
		}
		if strings.TrimSpace(route.Feature) == "" {
			return fmt.Errorf("extension %s route %q has no license feature", module, route.Prefix)
		}
		modulePrefix := "/" + module
		if route.Prefix != modulePrefix && !strings.HasPrefix(route.Prefix, modulePrefix+"/") {
			return fmt.Errorf("extension %s route %q must be namespaced beneath %s", module, route.Prefix, modulePrefix)
		}
		switch route.Scope {
		case RoutePublic, RouteAccount, RouteInstanceAdmin:
			if route.Permission != "" {
				return fmt.Errorf("extension %s route %q declares a permission outside organization scope", module, route.Prefix)
			}
		case RouteOrganization:
			if strings.TrimSpace(route.Permission) == "" {
				return fmt.Errorf("extension %s organization route %q has no required permission", module, route.Prefix)
			}
		default:
			return fmt.Errorf("extension %s route %q has unknown scope %q", module, route.Prefix, route.Scope)
		}
	}
	for i, job := range contrib.Jobs {
		if strings.TrimSpace(job.Feature) == "" {
			return fmt.Errorf("extension %s job %d has no license feature", module, i)
		}
		if strings.TrimSpace(job.Definition.Type) == "" || job.Definition.Handler == nil {
			return fmt.Errorf("extension %s job %d is incomplete", module, i)
		}
	}
	for i, sink := range contrib.EventSinks {
		if strings.TrimSpace(sink.Feature) == "" || sink.Sink == nil {
			return fmt.Errorf("extension %s event sink %d is incomplete", module, i)
		}
	}
	for i, closer := range contrib.Closers {
		if closer == nil {
			return fmt.Errorf("extension %s closer %d is nil", module, i)
		}
	}
	return nil
}

func closeContributions(closers []io.Closer) {
	for i := len(closers) - 1; i >= 0; i-- {
		_ = closers[i].Close()
	}
}
