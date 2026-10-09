package supabase_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/supabase"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "supabase",
		Params:          engine.Params{"host": "db.project.supabase.co", "port": "5432", "database": "app_db", "username": "app_user"},
		Secrets:         engine.Secrets{"password": "supa@secret"},
		LegacyTLSDSN:    "postgresql://postgres:secret@db.project.supabase.co:5432/postgres?sslmode=require",
		ExpectedTLSMode: "require",
	})
}
