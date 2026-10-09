package sqlserver_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/sqlserver"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "sqlserver",
		Params:          engine.Params{"host": "sql.example.com", "port": "1433", "database": "app db", "username": "sa"},
		Secrets:         engine.Secrets{"password": "sql@secret"},
		LegacyTLSDSN:    "sqlserver://sa:secret@sql.example.com:1433?database=app&encrypt=true",
		ExpectedTLSMode: "true",
	})
}

func TestConnectionSpecAlias(t *testing.T) {
	if _, ok := engine.ConnectionSpecFor("mssql"); !ok {
		t.Fatal("mssql alias did not resolve a ConnectionSpec")
	}
}
