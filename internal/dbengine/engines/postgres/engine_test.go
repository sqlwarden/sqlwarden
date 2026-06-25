package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestPostgresEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "postgres")
	enginetest.RunConnectionContract(t, "postgres", dbengine.ConnectionConfig{DSN: testDSN, Driver: "postgres"})
}
