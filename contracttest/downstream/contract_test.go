package downstream

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/sqlwarden/app"
	"github.com/sqlwarden/authorization"
	"github.com/sqlwarden/distribution"
)

func TestPublicCompositionContract(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.DB.Driver = "sqlite"
	cfg.DB.DSN = t.TempDir() + "/sqlwarden.db"
	cfg.DB.Automigrate = true
	backend := cfg.Files.StorageBackends["local"]
	backend.RootDir = t.TempDir() + "/files"
	cfg.Files.StorageBackends["local"] = backend

	application, err := app.New(cfg, slog.Default(), app.WithDistribution(func(host distribution.HostServices) (distribution.Dependencies, error) {
		if host.DB == nil || host.Accounts == nil || host.Sessions == nil || host.Jobs == nil || host.Request == nil {
			t.Fatal("host services are incomplete")
		}
		return distribution.Dependencies{
			AuthorizationConstraint: authorization.ConstraintFunc(func(context.Context, authorization.Request) authorization.Decision {
				return authorization.Decision{Allowed: true}
			}),
			InstallRoutes: func(routes distribution.RouteMounts) {
				routes.Public.Get("/downstream", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			},
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	var _ http.Handler = application.Handler()
}
