package tidb_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/tidb"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "tidb",
		Params:          engine.Params{"host": "gateway.example.com", "port": "4000", "database": "app_db", "username": "root"},
		Secrets:         engine.Secrets{"password": "tidb@secret"},
		LegacyTLSDSN:    "root:secret@tcp(gateway.example.com:4000)/app?tls=true",
		ExpectedTLSMode: "true",
		MissingPortDSN:  "root:tidb@secret@tcp(gateway.example.com)/app_db",
		DefaultPort:     "4000",
	})
}
