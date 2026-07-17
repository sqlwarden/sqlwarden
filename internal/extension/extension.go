// Package extension defines the explicit composition seam through which
// optional build-time modules contribute behavior to the application.
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

	"github.com/sqlwarden/internal/capability"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/jobs"
)

var validModuleName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

const maxModuleNameLength = 63 - len("schema_migrations_")

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
// absolute path relative to that scope. Capability is always enforced by core.
// Organization routes must also declare the required organization permission.
type Route struct {
	Scope      RouteScope
	Prefix     string
	Capability string
	Permission string
	Handler    http.Handler
}

// Job is a centrally capability-gated job definition.
type Job struct {
	Capability string
	Definition jobs.Definition
}

// EventSink is a centrally capability-gated best-effort event consumer.
type EventSink struct {
	Capability string
	Sink       events.Sink
}

// Contributions are created after core runtime dependencies are ready.
// Closers are invoked in reverse order during application shutdown.
type Contributions struct {
	Routes     []Route
	Jobs       []Job
	EventSinks []EventSink
	Closers    []io.Closer
}

// BootstrapDeps are available while constructing the active capability gate.
// LookupEnv and Now make environment-backed configuration and expiry checks
// deterministic in tests without coupling extensions to web.Config.
type BootstrapDeps struct {
	DB        *database.DB
	Logger    *slog.Logger
	LookupEnv func(string) (string, bool)
	Now       func() time.Time
}

// RuntimeDeps are available when a module creates its runtime contributions.
type RuntimeDeps struct {
	DB           *database.DB
	Logger       *slog.Logger
	Capabilities capability.Gate
	Events       *events.Bus
}

type MigrationSource func(driver string) (fs.FS, bool)
type CapabilityFactory func(context.Context, BootstrapDeps) (capability.Gate, error)
type StartFunc func(context.Context, RuntimeDeps) (Contributions, error)

// Module is an explicit extension manifest. Optional functions may be
// nil, but names are unique and all returned contributions are validated.
type Module struct {
	Name              string
	Migrations        MigrationSource
	CapabilityFactory CapabilityFactory
	Start             StartFunc
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
	capabilityProviders := 0
	for _, module := range r.modules {
		if !validModuleName.MatchString(module.Name) {
			return fmt.Errorf("extension module name %q must match %s", module.Name, validModuleName)
		}
		if len(module.Name) > maxModuleNameLength {
			return fmt.Errorf("extension module name %q exceeds %d characters", module.Name, maxModuleNameLength)
		}
		if _, exists := seen[module.Name]; exists {
			return fmt.Errorf("extension module name %q is registered more than once", module.Name)
		}
		seen[module.Name] = struct{}{}
		if module.CapabilityFactory != nil {
			capabilityProviders++
		}
	}
	if capabilityProviders > 1 {
		return fmt.Errorf("extension registry has %d capability providers; exactly one is allowed", capabilityProviders)
	}
	return nil
}

// CapabilityGate constructs the registry's capability gate after the database
// is initialized. A registry without a provider enables no optional features.
func (r *Registry) CapabilityGate(ctx context.Context, deps BootstrapDeps) (capability.Gate, error) {
	if deps.LookupEnv == nil {
		deps.LookupEnv = os.LookupEnv
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	for _, module := range r.Modules() {
		if module.CapabilityFactory == nil {
			continue
		}
		gate, err := module.CapabilityFactory(ctx, deps)
		if err != nil {
			return nil, fmt.Errorf("extension %s capability gate: %w", module.Name, err)
		}
		if gate == nil {
			return nil, fmt.Errorf("extension %s returned a nil capability gate", module.Name)
		}
		return gate, nil
	}
	return capability.None(), nil
}

// Start initializes modules in registration order and validates every runtime
// contribution before any background worker or HTTP listener can use it.
func (r *Registry) Start(ctx context.Context, deps RuntimeDeps) (Contributions, error) {
	if err := r.Validate(); err != nil {
		return Contributions{}, err
	}
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
		key := mountedRouteKey(route)
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

func mountedRouteKey(route Route) string {
	switch route.Scope {
	case RoutePublic, RouteAccount:
		return "api:" + route.Prefix
	case RouteInstanceAdmin:
		return "instance:" + route.Prefix
	case RouteOrganization:
		return "organization:" + route.Prefix
	default:
		return string(route.Scope) + ":" + route.Prefix
	}
}

func validateContributions(module string, contrib Contributions) error {
	for i, route := range contrib.Routes {
		if route.Handler == nil {
			return fmt.Errorf("extension %s route %d has no handler", module, i)
		}
		if !strings.HasPrefix(route.Prefix, "/") || route.Prefix == "/" {
			return fmt.Errorf("extension %s route %d has invalid prefix %q", module, i, route.Prefix)
		}
		if strings.TrimSpace(route.Capability) == "" {
			return fmt.Errorf("extension %s route %q has no required capability", module, route.Prefix)
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
		if strings.TrimSpace(job.Capability) == "" {
			return fmt.Errorf("extension %s job %d has no required capability", module, i)
		}
		if strings.TrimSpace(job.Definition.Type) == "" || job.Definition.Handler == nil {
			return fmt.Errorf("extension %s job %d is incomplete", module, i)
		}
	}
	for i, sink := range contrib.EventSinks {
		if strings.TrimSpace(sink.Capability) == "" || sink.Sink == nil {
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
