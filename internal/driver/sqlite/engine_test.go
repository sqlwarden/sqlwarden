package sqlite

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestSQLiteEngineRegisteredAndConforms(t *testing.T) {
	eng, err := dbengine.New("sqlite")
	if err != nil {
		t.Fatalf("dbengine.New(sqlite): %v", err)
	}
	if eng.DisplayName() != "SQLite" || eng.Dialect() != dbengine.DialectSQLite {
		t.Fatalf("unexpected engine: name=%q dialect=%q", eng.DisplayName(), eng.Dialect())
	}
	enginetest.RunCapabilityContract(t, eng)

	caps := eng.Capabilities().Capabilities
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
