package sqlite

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestSQLiteEngineRegisteredAndConforms(t *testing.T) {
	set, ok := dbengine.Describe("sqlite")
	if !ok {
		t.Fatal("sqlite engine not registered")
	}
	if set.Engine.DisplayName != "SQLite" || set.Engine.Dialect != dbengine.DialectSQLite {
		t.Fatalf("unexpected engine: name=%q dialect=%q", set.Engine.DisplayName, set.Engine.Dialect)
	}
	enginetest.RunCapabilityContract(t, "sqlite")

	caps := set.Capabilities
	if !caps[dbengine.CapabilitySchemaCatalog] || !caps[dbengine.CapabilityQueryCursor] {
		t.Errorf("sqlite should report schema.catalog + query.cursor: %+v", caps)
	}
	for _, capability := range []dbengine.Capability{
		dbengine.CapabilitySQLParse,
		dbengine.CapabilitySQLClassify,
		dbengine.CapabilitySQLRewrite,
		dbengine.CapabilitySQLComplete,
	} {
		if caps[capability] {
			t.Errorf("%s must be false until implemented", capability)
		}
	}
}
