package sqlite

import "testing"

func TestSQLiteQuoteIdent(t *testing.T) {
	if got := sqliteQuoteIdent(`we"ird`); got != `"we""ird"` {
		t.Fatalf("got %q", got)
	}
}

func TestSQLiteQualify(t *testing.T) {
	if got := sqliteQualify("main", "users"); got != `"main"."users"` {
		t.Fatalf("got %q", got)
	}
	if got := sqliteQualify("", "users"); got != `"users"` {
		t.Fatalf("got %q", got)
	}
}

func TestSQLiteIsBareIdent(t *testing.T) {
	for _, ok := range []string{"users", "_x", "col1"} {
		if !sqliteIsBareIdent(ok) {
			t.Fatalf("%q should be bare", ok)
		}
	}
	for _, bad := range []string{"", "1col", "we ird", "select", "SELECT", "table"} {
		if sqliteIsBareIdent(bad) {
			t.Fatalf("%q should not be bare", bad)
		}
	}
}
