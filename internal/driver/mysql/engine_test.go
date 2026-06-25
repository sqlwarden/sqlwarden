package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestMySQLEngineContract(t *testing.T) {
	eng, err := dbengine.New("mysql")
	if err != nil {
		t.Fatalf("dbengine.New(mysql): %v", err)
	}
	enginetest.RunCapabilityContract(t, eng)
	enginetest.RunConnectionContract(t, eng, dbengine.ConnectionConfig{DSN: testDSN, Driver: "mysql"})
}
