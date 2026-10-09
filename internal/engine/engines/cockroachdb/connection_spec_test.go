package cockroachdb_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/cockroachdb"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "cockroachdb",
		Params:          engine.Params{"host": "cluster.example.com", "port": "26257", "database": "defaultdb", "username": "root"},
		Secrets:         engine.Secrets{"password": "roach@secret"},
		LegacyTLSDSN:    "postgresql://root:secret@cluster.example.com:26257/defaultdb?sslmode=verify-full",
		ExpectedTLSMode: "verify-full",
	})
}
