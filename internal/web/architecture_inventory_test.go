package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestArchitectureRouteInventory pins the complete method and path surface.
// Behavioral handler tests own request and response semantics. This snapshot
// catches a route that is dropped or remapped when handlers move between
// packages. When a route is added or removed on purpose, update both values.
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
		wantCount  = 297
		wantDigest = "97de94001a0447d0ca2062de2687ce8f445e443c7d5ae7a0e9afcb0c56309fbb"
	)
	if len(routes) != wantCount || digest != wantDigest {
		t.Fatalf("route inventory count=%d digest=%s, want count=%d digest=%s\n%s",
			len(routes), digest, wantCount, wantDigest, strings.Join(routes, "\n"))
	}
}
