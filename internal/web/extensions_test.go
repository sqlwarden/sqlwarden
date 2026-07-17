package web

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/license"
)

type testLicense struct{ licensed bool }

func testDBDriver() string {
	driver := os.Getenv("TEST_DB_DRIVER")
	if driver == "" {
		return "postgres"
	}
	return driver
}

func (s testLicense) Edition() string        { return "enterprise" }
func (s testLicense) IsLicensed(string) bool { return s.licensed }
func (s testLicense) LicensedFeatures() []string {
	if s.licensed {
		return []string{"faketest"}
	}
	return nil
}
func (s testLicense) Require(string) error {
	if !s.licensed {
		return license.ErrNotLicensed
	}
	return nil
}

func fakeModule(scope extension.RouteScope) extension.Module {
	return extension.Module{
		Name: "faketest",
		Migrations: func(string) (fs.FS, bool) {
			return fstest.MapFS{
				"000001_fake.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE ee_faketest (id INTEGER PRIMARY KEY, note TEXT);")},
				"000001_fake.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE ee_faketest;")},
			}, true
		},
		Start: func(context.Context, extension.RuntimeDeps) (extension.Contributions, error) {
			r := chi.NewRouter()
			r.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			return extension.Contributions{Routes: []extension.Route{{
				Scope: scope, Prefix: "/faketest/ping", Feature: "faketest", Handler: r,
			}}}, nil
		},
	}
}

func attachTestModule(t *testing.T, app *application, module extension.Module) {
	t.Helper()
	reg := extension.NewRegistry()
	reg.Add(module)
	if err := reg.Validate(); err != nil {
		t.Fatal(err)
	}
	contrib, err := reg.Start(context.Background(), extension.RuntimeDeps{
		DB: app.db, Logger: app.logger, License: app.licenseService, Events: app.eventBus,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.extensionRoutes = append(app.extensionRoutes, contrib.Routes...)
}

func TestExtensionAccountRouteRequiresAuthentication(t *testing.T) {
	app := newTestApplication(t)
	app.licenseService = testLicense{licensed: true}
	attachTestModule(t, app, fakeModule(extension.RouteAccount))

	res := send(t, newTestRequest(t, http.MethodGet, "/api/v1/faketest/ping", nil), app.routes())
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous account-scoped route status = %d, want 401", res.StatusCode)
	}
}

func TestExtensionRouteIsCentrallyLicenseGated(t *testing.T) {
	app := newTestApplication(t)
	app.licenseService = testLicense{licensed: false}
	attachTestModule(t, app, fakeModule(extension.RoutePublic))

	res := send(t, newTestRequest(t, http.MethodGet, "/api/v1/faketest/ping", nil), app.routes())
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("unlicensed route status = %d, want 403", res.StatusCode)
	}
	errorBody, _ := res.BodyFields["error"].(map[string]any)
	if code, _ := errorBody["code"].(string); code != license.CodeRequired {
		t.Fatalf("error code = %q, want %q", code, license.CodeRequired)
	}
}

func TestRunExtensionMigrations(t *testing.T) {
	app := newTestApplication(t)
	reg := extension.NewRegistry()
	reg.Add(fakeModule(extension.RoutePublic))

	if err := runExtensionMigrations(app.db, reg, testDBDriver(), app.logger); err != nil {
		t.Fatal(err)
	}

	var count int
	err := app.db.NewSelect().Table("ee_faketest").ColumnExpr("count(*)").Scan(context.Background(), &count)
	if err != nil {
		t.Fatalf("expected ee_faketest table after extension migrations: %v", err)
	}
}

func TestRegistryLicenseServiceDefaultsToCommunity(t *testing.T) {
	svc, err := extension.NewRegistry().LicenseService(context.Background(), extension.BootstrapDeps{
		LookupEnv: os.LookupEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.Edition() != "community" {
		t.Fatalf("Edition() = %q, want community", svc.Edition())
	}
}

func TestExtensionRoutesDoNotBypassInstanceAdmin(t *testing.T) {
	app := newTestApplication(t)
	app.licenseService = testLicense{licensed: true}
	attachTestModule(t, app, fakeModule(extension.RouteInstanceAdmin))

	srv := httptest.NewServer(app.routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/v1/instance/faketest/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous instance-admin route status = %d, want 401", resp.StatusCode)
	}
}

func TestExtensionJobsAndSinksAreCentrallyLicenseGated(t *testing.T) {
	app := newTestApplication(t)
	app.licenseService = testLicense{licensed: false}

	jobRan := false
	handler := app.licensedJobHandler("faketest", jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
		jobRan = true
		return nil, nil
	}))
	if _, err := handler.Handle(context.Background(), jobs.Runtime{}); err == nil {
		t.Fatal("expected unlicensed job to be rejected")
	}
	if jobRan {
		t.Fatal("unlicensed job handler ran")
	}

	capture := &testCaptureSink{}
	sink := app.licensedEventSink("faketest", capture)
	sink.HandleEvent(context.Background(), events.Event{Action: "test", Outcome: "success"})
	if _, found := capture.find("test", "success"); found {
		t.Fatal("unlicensed event sink received an event")
	}
}
