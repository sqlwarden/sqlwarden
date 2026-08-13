package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestMySQLEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "mysql")
	set, ok := engine.Describe("mysql")
	if !ok {
		t.Fatal("mysql engine not registered")
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
	enginetest.RunConnectionContract(t, "mysql", engine.ConnectionConfig{DSN: testDSN, Driver: "mysql"})
}
