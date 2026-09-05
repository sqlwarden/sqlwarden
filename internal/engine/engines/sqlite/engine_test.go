package sqlite

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestSQLiteEngineRegisteredAndConforms(t *testing.T) {
	set, ok := engine.Describe("sqlite")
	if !ok {
		t.Fatal("sqlite engine not registered")
	}
	if set.Engine.DisplayName != "SQLite" || set.Engine.Dialect != engine.DialectSQLite {
		t.Fatalf("unexpected engine: name=%q dialect=%q", set.Engine.DisplayName, set.Engine.Dialect)
	}
	enginetest.RunCapabilityContract(t, "sqlite")

	caps := set.Capabilities
	if !caps[engine.CapabilitySchemaDirectory] || !caps[engine.CapabilityQueryCursor] {
		t.Errorf("sqlite should report schema.directory + query.cursor: %+v", caps)
	}
	for _, capability := range []engine.Capability{
		engine.CapabilitySQLParse,
		engine.CapabilitySQLClassify,
		engine.CapabilitySQLComplete,
		engine.CapabilitySQLSafetyCheck,
		engine.CapabilitySQLExplain,
	} {
		if !caps[capability] {
			t.Errorf("%s must be true", capability)
		}
	}
	if caps[engine.CapabilitySQLRewrite] {
		t.Errorf("sql.rewrite must be false until implemented")
	}
}
