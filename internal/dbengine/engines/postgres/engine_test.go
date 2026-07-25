package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestPostgresEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "postgres")
	set, ok := dbengine.Describe("postgres")
	if !ok {
		t.Fatal("postgres engine not registered")
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
	enginetest.RunConnectionContract(t, "postgres", dbengine.ConnectionConfig{DSN: testDSN, Driver: "postgres"})
}
