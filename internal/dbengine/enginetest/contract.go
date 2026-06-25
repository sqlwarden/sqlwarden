// Package enginetest provides a reusable conformance suite that every dbengine
// engine must satisfy. It imports testing on purpose, like net/http/httptest:
// engine packages call these from their own _test.go files.
package enginetest

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine"
)

// knownCapabilities is the closed set of capability keys an engine may report.
var knownCapabilities = map[dbengine.Capability]bool{
	dbengine.CapabilitySchemaCatalog: true,
	dbengine.CapabilitySchemaObjects: true,
	dbengine.CapabilityQueryCursor:   true,
	dbengine.CapabilitySQLParse:      true,
	dbengine.CapabilitySQLClassify:   true,
	dbengine.CapabilitySQLRewrite:    true,
	dbengine.CapabilitySQLComplete:   true,
}

// RunCapabilityContract asserts the static-capability invariants every engine
// must satisfy. It MUST pass without opening a target connection.
func RunCapabilityContract(t *testing.T, eng dbengine.Engine) {
	t.Helper()
	if eng.ID() == "" {
		t.Fatal("engine ID must not be empty")
	}
	if eng.Dialect() == "" {
		t.Fatal("engine Dialect must not be empty")
	}
	set := eng.Capabilities()
	if set.Engine.ID != eng.ID() {
		t.Fatalf("descriptor ID %q != engine ID %q", set.Engine.ID, eng.ID())
	}
	for capability := range set.Capabilities {
		if !knownCapabilities[capability] {
			t.Fatalf("engine reports unknown capability key %q", capability)
		}
	}
	if set.Capabilities[dbengine.CapabilitySchemaCatalog] && set.Schema == nil {
		t.Fatal("schema.catalog capability set but Schema spec is nil")
	}
	if !set.Capabilities[dbengine.CapabilitySchemaCatalog] && set.Schema != nil {
		t.Fatal("Schema spec present but schema.catalog capability is false")
	}
}

// RunConnectionContract opens a real connection and verifies the adapter
// round-trips a trivial query. Callers supply a live DSN (e.g. a testcontainer).
func RunConnectionContract(t *testing.T, eng dbengine.Engine, cfg dbengine.ConnectionConfig) {
	t.Helper()
	conn, err := eng.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	rs, err := conn.Query(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Query SELECT 1: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("SELECT 1 returned %d rows, want 1", len(rs.Rows))
	}
}
