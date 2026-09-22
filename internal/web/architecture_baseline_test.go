package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	coreapp "github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/config"
)

// TestArchitectureRouteInventory characterizes the complete HTTP method/path
// surface before handlers move into application services. Behavioral handler
// tests remain the contract for request and response semantics; this snapshot
// catches an endpoint that is accidentally dropped or remapped during a move.
func TestArchitectureRouteInventory(t *testing.T) {
	app := newTestApplication(t)
	router, ok := app.routes().(chi.Routes)
	if !ok {
		t.Fatal("application router does not expose chi routes")
	}
	var routes []string
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(routes)
	sum := sha256.Sum256([]byte(strings.Join(routes, "\n")))
	digest := hex.EncodeToString(sum[:])

	const (
		wantCount  = 308
		wantDigest = "d21ae2f92f742a5bd049e55bbc869a138804a885c795984dc2ac9351d203c621"
	)
	if len(routes) != wantCount || digest != wantDigest {
		t.Fatalf("route inventory count=%d digest=%s, want count=%d digest=%s", len(routes), digest, wantCount, wantDigest)
	}
}

// BenchmarkArchitectureAPISetupStatus is the Phase 0 baseline for a small
// public API read through the production router and middleware stack.
func BenchmarkArchitectureAPISetupStatus(b *testing.B) {
	cfg := config.Default()
	cfg.DB.DSN = filepath.Join(b.TempDir(), "baseline.db")
	cfg.DB.Automigrate = true
	cfg.Files.StorageBackends[config.DefaultFilesActiveBackend] = config.FileStorageBackend{
		Type: config.FilesStorageBackendFilesystem, RootDir: b.TempDir(),
	}
	built, err := coreapp.Build(context.Background(), coreapp.Options{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = built.Close(context.Background()) })
	handler := NewApplication(built.Services).Handler()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		request := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			b.Fatalf("status = %d", response.Code)
		}
	}
}
