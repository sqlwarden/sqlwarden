package mariadb_test

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/mariadb"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver:          "mariadb",
		Params:          engine.Params{"host": "db.example.com", "port": "3306", "database": "app_db", "username": "app_user"},
		Secrets:         engine.Secrets{"password": "maria@secret"},
		LegacyTLSDSN:    "app:secret@tcp(db.example.com:3306)/app?tls=skip-verify",
		ExpectedTLSMode: "skip-verify",
		MissingPortDSN:  "app_user:maria@secret@tcp(db.example.com)/app_db",
		DefaultPort:     "3306",
	})
}
