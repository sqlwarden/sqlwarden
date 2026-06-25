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
	if !caps[dbengine.CapabilitySQLParse] || !caps[dbengine.CapabilitySQLClassify] || !caps[dbengine.CapabilitySQLRewrite] {
		t.Errorf("sqlite should report gosqlx parse/classify/rewrite: %+v", caps)
	}
	if caps[dbengine.CapabilitySQLComplete] {
		t.Errorf("sql.complete must be false until completion is implemented")
	}
}
