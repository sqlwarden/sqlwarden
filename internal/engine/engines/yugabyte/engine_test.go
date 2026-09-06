package yugabyte

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestYugabyteEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "yugabyte")
	set, ok := engine.Describe("yugabyte")
	if !ok {
		t.Fatal("yugabyte engine not registered")
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
	enginetest.RunConnectionContract(t, "yugabyte", engine.ConnectionConfig{DSN: testDSN, Driver: "yugabyte"})
}
