package postgres_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver: "postgres",
		Params: engine.Params{
			"host": "db.example.com", "port": "5432", "database": "app db", "username": "app user",
		},
		Secrets:         engine.Secrets{"password": "p@ss:/ word"},
		LegacyTLSDSN:    "postgresql://app:secret@db.example.com:5432/app?sslmode=verify-full",
		ExpectedTLSMode: "verify-full",
	})
}

func TestConnectionSpecAlias(t *testing.T) {
	if _, ok := engine.ConnectionSpecFor("postgresql"); !ok {
		t.Fatal("postgresql alias did not resolve a ConnectionSpec")
	}
}
