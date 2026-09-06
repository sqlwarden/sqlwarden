package mariadb

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestMariaDBEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "mariadb")
	set, ok := engine.Describe("mariadb")
	if !ok {
		t.Fatal("mariadb engine not registered")
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
	enginetest.RunConnectionContract(t, "mariadb", engine.ConnectionConfig{DSN: testDSN, Driver: "mariadb"})
}
