package neon_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/neon"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "neon",
		Params:          engine.Params{"host": "ep-example.neon.tech", "port": "5432", "database": "neondb", "username": "neondb_owner"},
		Secrets:         engine.Secrets{"password": "neon@secret"},
		LegacyTLSDSN:    "postgresql://owner:secret@ep-example.neon.tech:5432/neondb?sslmode=require",
		ExpectedTLSMode: "require",
	})
}
