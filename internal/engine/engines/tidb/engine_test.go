package tidb

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestTiDBEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "tidb")
	set, ok := engine.Describe("tidb")
	if !ok {
		t.Fatal("tidb engine not registered")
	}
	for _, capability := range []engine.Capability{
		engine.CapabilitySQLParse,
		engine.CapabilitySQLClassify,
		engine.CapabilitySQLComplete,
	} {
		if !set.Capabilities[capability] {
			t.Errorf("%s must be true", capability)
		}
	}
	for _, capability := range []engine.Capability{
		engine.CapabilitySQLRewrite,
	} {
		if set.Capabilities[capability] {
			t.Errorf("%s must remain false", capability)
		}
	}
	enginetest.RunConnectionContract(t, "tidb", engine.ConnectionConfig{DSN: testDSN, Driver: "tidb"})
}
