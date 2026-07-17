package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/capability"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
)

func (app *application) mountExtensionRoutes(r chi.Router, scope extension.RouteScope) {
	for _, route := range app.extensionRoutes {
		if route.Scope != scope {
			continue
		}
		handler := route.Handler
		if route.Access == extension.RouteAccessCapability {
			handler = app.requireCapability(route.Capability)(handler)
		}
		if scope == extension.RouteOrganization {
			handler = app.requireOrgPermission(route.Permission)(handler)
		}
		app.logger.Info("mounting extension route",
			"scope", route.Scope,
			"prefix", route.Prefix,
			"access", route.Access,
			"capability", route.Capability,
		)
		r.Mount(route.Prefix, handler)
	}
}

func (app *application) requireCapability(name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := app.capabilityGate.Require(name); err != nil {
				app.logInfo(r, "optional capability unavailable", slog.String("capability", name))
				_ = extension.WriteCapabilityUnavailable(w, name)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (app *application) capabilityGatedJobHandler(name string, next jobs.Handler) jobs.Handler {
	return jobs.HandlerFunc(func(ctx context.Context, runtime jobs.Runtime) (any, error) {
		if err := app.capabilityGate.Require(name); err != nil {
			return nil, jobs.Permanent(capability.CodeUnavailable, "This job capability is unavailable.")
		}
		return next.Handle(ctx, runtime)
	})
}

type capabilityGatedEventSink struct {
	gate capability.Gate
	name string
	next events.Sink
}

func (app *application) capabilityGatedEventSink(name string, sink events.Sink) events.Sink {
	return capabilityGatedEventSink{gate: app.capabilityGate, name: name, next: sink}
}

func (s capabilityGatedEventSink) HandleEvent(ctx context.Context, event events.Event) error {
	if s.gate.Require(s.name) != nil {
		return nil
	}
	return s.next.HandleEvent(ctx, event)
}
