package mysql_test

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/mysql"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver: "mysql",
		Params: engine.Params{
			"host": "db.example.com", "port": "3306", "database": "app_db", "username": "app_user",
		},
		Secrets:         engine.Secrets{"password": "p@ss:/ word"},
		LegacyTLSDSN:    "app:secret@tcp(db.example.com:3306)/app?tls=true",
		ExpectedTLSMode: "true",
		MissingPortDSN:  "app_user:p@ss:/ word@tcp(db.example.com)/app_db",
		DefaultPort:     "3306",
	})
}

func TestConnectionSpecRejectsDatabaseDelimiters(t *testing.T) {
	spec, ok := engine.ConnectionSpecFor("mysql")
	if !ok {
		t.Fatal("mysql ConnectionSpec not found")
	}
	for _, database := range []string{"app/name", "app?name"} {
		params := engine.Params{"host": "db.example.com", "port": "3306", "database": database, "username": "app"}
		if _, err := spec.BuildDSN(params, nil); err == nil {
			t.Errorf("BuildDSN accepted database %q", database)
		} else if strings.Contains(err.Error(), database) {
			t.Error("BuildDSN error echoed the database name")
		}
	}
}

func TestConnectionSpecRejectsEmptyTCPHost(t *testing.T) {
	spec, ok := engine.ConnectionSpecFor("mysql")
	if !ok {
		t.Fatal("mysql ConnectionSpec not found")
	}
	const dsn = "app:secret@tcp()/app_db"
	if _, _, err := spec.ParseDSN(dsn); err == nil {
		t.Fatal("ParseDSN accepted an empty TCP host")
	} else if strings.Contains(err.Error(), dsn) {
		t.Error("ParseDSN error echoed the DSN")
	}
}
