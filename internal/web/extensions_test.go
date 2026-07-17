package web

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/events"
	"github.com/sqlwarden/internal/extension"
	"github.com/sqlwarden/internal/jobs"
)

type fakeExtension struct {
	mu         sync.Mutex
	sinkEvents []events.Event
}

func (f *fakeExtension) Name() string { return "faketest" }

func (f *fakeExtension) Migrations(string) (fs.FS, bool) {
	return fstest.MapFS{
		"000001_fake.up.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE ee_faketest (id INTEGER PRIMARY KEY, note TEXT);"),
		},
		"000001_fake.down.sql": &fstest.MapFile{
			Data: []byte("DROP TABLE ee_faketest;"),
		},
	}, true
}

func (f *fakeExtension) RegisterRoutes(r chi.Router, _ *extension.Deps) {
	r.Get("/api/v1/faketest/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (f *fakeExtension) Jobs(*extension.Deps) []jobs.Definition {
	return []jobs.Definition{{
		Type:        "faketest_noop",
		MaxAttempts: 1,
		Handler: jobs.HandlerFunc(func(context.Context, jobs.Runtime) (any, error) {
			return nil, nil
		}),
	}}
}

func (f *fakeExtension) EventSink(*extension.Deps) events.Sink { return f }

func (f *fakeExtension) HandleEvent(_ context.Context, ev events.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sinkEvents = append(f.sinkEvents, ev)
}

func testDBDriver() string {
	driver := os.Getenv("TEST_DB_DRIVER")
	if driver == "" {
		driver = "postgres"
	}
	return driver
}

func TestExtensionRoutesAreMounted(t *testing.T) {
	app := newTestApplication(t)
	fake := &fakeExtension{}
	app.extensions.Add(fake)

	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/faketest/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("extension route status = %d, want 200", resp.StatusCode)
	}
}

func TestRunExtensionMigrations(t *testing.T) {
	app := newTestApplication(t)
	reg := extension.NewRegistry()
	reg.Add(&fakeExtension{})

	if err := runExtensionMigrations(app.db, reg, testDBDriver(), app.logger); err != nil {
		t.Fatal(err)
	}

	// Table existence is asserted by querying it, which works on both
	// postgres and sqlite test databases.
	var count int
	err := app.db.NewSelect().Table("ee_faketest").ColumnExpr("count(*)").Scan(context.Background(), &count)
	if err != nil {
		t.Fatalf("expected ee_faketest table after extension migrations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected empty ee_faketest table, got %d rows", count)
	}
}

func TestLicenseServiceForDefaultsToCommunity(t *testing.T) {
	svc := licenseServiceFor(extension.NewRegistry())
	if svc.Edition() != "community" {
		t.Fatalf("Edition() = %q, want community", svc.Edition())
	}
}
