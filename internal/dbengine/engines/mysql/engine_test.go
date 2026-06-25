package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
)

func TestMySQLEngineContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "mysql")
	enginetest.RunConnectionContract(t, "mysql", dbengine.ConnectionConfig{DSN: testDSN, Driver: "mysql"})
}
