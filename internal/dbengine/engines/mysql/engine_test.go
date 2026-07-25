package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestMySQLEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "mysql")
	set, ok := dbengine.Describe("mysql")
	if !ok {
		t.Fatal("mysql engine not registered")
	}
	for _, capability := range []dbengine.Capability{
		dbengine.CapabilitySQLParse,
		dbengine.CapabilitySQLClassify,
	} {
		if !set.Capabilities[capability] {
			t.Errorf("%s must be true", capability)
		}
	}
	for _, capability := range []dbengine.Capability{
		dbengine.CapabilitySQLRewrite,
		dbengine.CapabilitySQLComplete,
	} {
		if set.Capabilities[capability] {
			t.Errorf("%s must remain false", capability)
		}
	}
	enginetest.RunConnectionContract(t, "mysql", dbengine.ConnectionConfig{DSN: testDSN, Driver: "mysql"})
}
