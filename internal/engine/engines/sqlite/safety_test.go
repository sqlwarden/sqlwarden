package sqlite

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/safety"
)

func TestSQLiteSafetyCheck(t *testing.T) {
	cases := []struct {
		name       string
		sql        string
		wantUnsafe bool
		wantSource string
	}{
		{"update no where", "UPDATE t SET a = 1", true, "rqlite"},
		{"delete no where", "DELETE FROM t", true, "rqlite"},
		{"update with where", "UPDATE t SET a = 1 WHERE id = 2", false, "rqlite"},
		{"delete with where", "DELETE FROM t WHERE id = 2", false, "rqlite"},
		{"delete with limit", "DELETE FROM t LIMIT 10", false, "rqlite"},
		{"select", "SELECT * FROM t", false, "rqlite"},
		{"non-dml parse failure", "!!! not sql", false, "heuristic"},
		{"multi one unsafe", "DELETE FROM t WHERE id = 1; UPDATE t SET a = 2", true, "rqlite"},
		// rqlite/sql cannot parse VACUUM, so the whole batch fails to parse; the
		// heuristic fallback must still raise the bare DELETE's missing-WHERE rail.
		{"unparseable batch keeps missing-where rail", "DELETE FROM t; VACUUM", true, "heuristic"},
	}
	d := &sqliteDriver{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := d.Check(context.Background(), safety.Request{SQL: tc.sql})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if res.Unsafe != tc.wantUnsafe {
				t.Fatalf("Unsafe = %v, want %v (%+v)", res.Unsafe, tc.wantUnsafe, res.Statements)
			}
			if res.Source != tc.wantSource {
				t.Fatalf("Source = %q, want %q", res.Source, tc.wantSource)
			}
			if tc.wantUnsafe {
				for _, s := range res.Statements {
					if s.Kind != safety.KindUnsafeMissingWhere {
						t.Fatalf("Kind = %v, want KindUnsafeMissingWhere", s.Kind)
					}
				}
			}
		})
	}
}

func TestSQLiteSafetySpans(t *testing.T) {
	sql := "SELECT 1; DELETE FROM t"
	res, err := (&sqliteDriver{}).Check(context.Background(), safety.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(res.Statements) != 1 {
		t.Fatalf("Statements = %+v, want 1", res.Statements)
	}
	if got := sql[res.Statements[0].StartOffset:res.Statements[0].EndOffset]; got != "DELETE FROM t" {
		t.Fatalf("span text = %q", got)
	}
}
