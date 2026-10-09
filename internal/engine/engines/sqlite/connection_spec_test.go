package sqlite_test

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/connectionspectest"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
)

func TestConnectionSpec(t *testing.T) {
	connectionspectest.Run(t, connectionspectest.Case{
		Driver: "sqlite", Params: engine.Params{"path": "/var/lib/app data.db"}, Secrets: engine.Secrets{},
	})
}

func TestConnectionSpecAlias(t *testing.T) {
	if _, ok := engine.ConnectionSpecFor("sqlite3"); !ok {
		t.Fatal("sqlite3 alias did not resolve a ConnectionSpec")
	}
}

func TestConnectionSpecRejectsPathDelimiters(t *testing.T) {
	spec, ok := engine.ConnectionSpecFor("sqlite")
	if !ok {
		t.Fatal("sqlite ConnectionSpec not found")
	}
	for _, path := range []string{"/tmp/app?mode=rw.db", "/tmp/app#copy.db"} {
		if _, err := spec.BuildDSN(engine.Params{"path": path}, nil); err == nil {
			t.Errorf("BuildDSN accepted path %q", path)
		} else if strings.Contains(err.Error(), path) {
			t.Error("BuildDSN error echoed the path")
		}
	}
}

func TestConnectionSpecNormalizesAbsoluteFileURI(t *testing.T) {
	spec, ok := engine.ConnectionSpecFor("sqlite")
	if !ok {
		t.Fatal("sqlite ConnectionSpec not found")
	}
	params, _, err := spec.ParseDSN("file:///var/lib/app.db")
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if got := params["path"]; got != "/var/lib/app.db" {
		t.Errorf("path = %q, want /var/lib/app.db", got)
	}
}

func TestConnectionSpecPercentRoundTrips(t *testing.T) {
	spec, ok := engine.ConnectionSpecFor("sqlite")
	if !ok {
		t.Fatal("sqlite ConnectionSpec not found")
	}
	for _, tc := range []struct {
		path string
		dsn  string
	}{
		{path: "/tmp/100%.db", dsn: "file:/tmp/100%25.db"},
		{path: "/tmp/a%20b.db", dsn: "file:/tmp/a%2520b.db"},
	} {
		dsn, err := spec.BuildDSN(engine.Params{"path": tc.path}, nil)
		if err != nil {
			t.Fatalf("BuildDSN(%q): %v", tc.path, err)
		}
		if dsn != tc.dsn {
			t.Errorf("DSN = %q, want %q", dsn, tc.dsn)
		}
		params, _, err := spec.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("ParseDSN for %q: %v", tc.path, err)
		}
		if got := params["path"]; got != tc.path {
			t.Errorf("path = %q, want %q", got, tc.path)
		}
	}
}
