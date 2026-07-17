package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
)

func (app *application) mountExtensionRoutes(r chi.Router, scope extension.RouteScope) {
	for _, route := range app.extensionRoutes {
		if route.Scope != scope {
			continue
		}
		handler := app.requireLicense(route.Feature)(route.Handler)
		if scope == extension.RouteOrganization {
			handler = app.requireOrgPermission(route.Permission)(handler)
		}
		app.logger.Info("mounting extension route",
			"scope", route.Scope,
			"prefix", route.Prefix,
			"feature", route.Feature,
		)
		r.Mount(route.Prefix, handler)
	}
}

func (app *application) requireLicense(feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := app.licenseService.Require(feature); err != nil {
				app.logInfo(r, "enterprise feature denied without license", slog.String("feature", feature))
				_ = extension.WriteLicenseRequired(w, feature)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (app *application) licensedJobHandler(feature string, next jobs.Handler) jobs.Handler {
	return jobs.HandlerFunc(func(ctx context.Context, runtime jobs.Runtime) (any, error) {
		if err := app.licenseService.Require(feature); err != nil {
			return nil, jobs.Permanent(license.CodeRequired, "Enterprise license is not active for this job.")
		}
		return next.Handle(ctx, runtime)
	})
}

type licenseGatedEventSink struct {
	service license.Service
	feature string
	next    events.Sink
}

func (app *application) licensedEventSink(feature string, sink events.Sink) events.Sink {
	return licenseGatedEventSink{service: app.licenseService, feature: feature, next: sink}
}

func (s licenseGatedEventSink) HandleEvent(ctx context.Context, event events.Event) error {
	if s.service.Require(s.feature) != nil {
		return nil
	}
	return s.next.HandleEvent(ctx, event)
}
