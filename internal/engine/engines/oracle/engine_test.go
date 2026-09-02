package oracle

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/explain"
)

func TestOracleEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "oracle")

	set, ok := engine.Describe("oracle")
	if !ok {
		t.Fatal("oracle engine not registered")
	}
	if set.Engine.DisplayName != "Oracle" {
		t.Errorf("DisplayName = %q, want %q", set.Engine.DisplayName, "Oracle")
	}
	if set.Engine.Dialect != engine.DialectOracle {
		t.Errorf("Dialect = %q, want %q", set.Engine.Dialect, engine.DialectOracle)
	}
	for _, capability := range []engine.Capability{
		engine.CapabilitySchemaDirectory,
		engine.CapabilitySchemaObjects,
		engine.CapabilityDDL,
		engine.CapabilityQueryCursor,
		engine.CapabilitySQLSafetyCheck,
		engine.CapabilitySQLGenerate,
		engine.CapabilitySQLParse,
		engine.CapabilitySQLClassify,
		engine.CapabilitySQLComplete,
	} {
		if !set.Capabilities[capability] {
			t.Errorf("%s must be true", capability)
		}
	}
	if set.Capabilities[engine.CapabilitySQLExplain] {
		t.Error("sql.explain must remain false for oracle")
	}
	if set.Capabilities[engine.CapabilitySQLRewrite] {
		t.Error("sql.rewrite must remain false for oracle")
	}
}

func TestOracleDriverImplementsCapabilities(t *testing.T) {
	var d engine.Driver = &oracleDriver{}
	if _, ok := d.(explain.Explainer); ok {
		t.Error("oracle must NOT implement explain.Explainer")
	}
}
