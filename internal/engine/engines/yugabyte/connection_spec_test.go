package yugabyte_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/yugabyte"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "yugabyte",
		Params:          engine.Params{"host": "yb.example.com", "port": "5433", "database": "yugabyte", "username": "yugabyte"},
		Secrets:         engine.Secrets{"password": "yb@secret"},
		LegacyTLSDSN:    "postgresql://yugabyte:secret@yb.example.com:5433/yugabyte?sslmode=require",
		ExpectedTLSMode: "require",
	})
}
