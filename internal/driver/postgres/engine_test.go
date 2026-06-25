package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestPostgresEngineContract(t *testing.T) {
	eng, err := dbengine.New("postgres")
	if err != nil {
		t.Fatalf("dbengine.New(postgres): %v", err)
	}
	enginetest.RunCapabilityContract(t, eng)
	enginetest.RunConnectionContract(t, eng, dbengine.ConnectionConfig{DSN: testDSN, Driver: "postgres"})
}
