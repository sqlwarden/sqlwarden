// Package enginetest provides a reusable conformance suite that every registered
// engine must satisfy. It imports testing on purpose, like net/http/httptest:
// engine packages call these from their own _test.go files.
package enginetest

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

// knownCapabilities is the closed set of capability keys an engine may report.
var knownCapabilities = map[engine.Capability]bool{
	engine.CapabilitySchemaDirectory: true,
	engine.CapabilitySchemaObjects:   true,
	engine.CapabilityDDL:             true,
	engine.CapabilityQueryCursor:     true,
	engine.CapabilitySQLParse:        true,
	engine.CapabilitySQLClassify:     true,
	engine.CapabilitySQLRewrite:      true,
	engine.CapabilitySQLComplete:     true,
}

// RunCapabilityContract asserts the static-capability invariants every engine
// must satisfy. It must pass without opening a target connection.
func RunCapabilityContract(t *testing.T, name string) {
	t.Helper()
	set, ok := engine.Describe(name)
	if !ok {
		t.Fatalf("engine %q is not registered", name)
	}
	if set.Engine.ID == "" {
		t.Fatal("engine ID must not be empty")
	}
	if set.Engine.Dialect == "" {
		t.Fatal("engine Dialect must not be empty")
	}
	for capability := range set.Capabilities {
		if !knownCapabilities[capability] {
			t.Fatalf("engine reports unknown capability key %q", capability)
		}
	}
	if set.Capabilities[engine.CapabilitySchemaDirectory] && set.Schema == nil {
		t.Fatal("schema.directory capability set but Schema spec is nil")
	}
	if !set.Capabilities[engine.CapabilitySchemaDirectory] && set.Schema != nil {
		t.Fatal("Schema spec present but schema.directory capability is false")
	}
	if set.Capabilities[engine.CapabilityDDL] != (set.DDL != nil) {
		t.Fatal("schema.edit capability and schema edit spec disagree")
	}
}

// RunConnectionContract opens a real connection — New + Connect — and verifies a
// trivial query round-trips. Callers supply a live DSN (e.g. a testcontainer).
func RunConnectionContract(t *testing.T, name string, cfg engine.ConnectionConfig) {
	t.Helper()
	d, err := engine.New(name)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := d.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	rs, err := d.Query(context.Background(), "SELECT 1")
	if err != nil {
		t.Fatalf("Query SELECT 1: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("SELECT 1 returned %d rows, want 1", len(rs.Rows))
	}
}
