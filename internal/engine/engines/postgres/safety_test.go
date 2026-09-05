package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/safety"
)

func TestPostgresSafetyCheck(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		unsafe    bool
		wantCount int
	}{
		{name: "bare update", sql: "UPDATE widgets SET active = false", unsafe: true, wantCount: 1},
		{name: "bare delete", sql: "DELETE FROM widgets", unsafe: true, wantCount: 1},
		{name: "update with where", sql: "UPDATE widgets SET active = false WHERE id = 1", unsafe: false, wantCount: 0},
		{name: "delete with where", sql: "DELETE FROM widgets WHERE id = 1", unsafe: false, wantCount: 0},
		{name: "select is never flagged", sql: "SELECT * FROM widgets", unsafe: false, wantCount: 0},
		{name: "create is never flagged", sql: "CREATE TABLE widgets(id bigint)", unsafe: false, wantCount: 0},
		{
			name:      "multi-statement mixes safe and unsafe",
			sql:       "UPDATE widgets SET active = false WHERE id = 1; DELETE FROM widgets",
			unsafe:    true,
			wantCount: 1,
		},
		{
			name:      "both statements unsafe reports both offsets",
			sql:       "UPDATE widgets SET active = false; DELETE FROM widgets",
			unsafe:    true,
			wantCount: 2,
		},
	}
	d := &Driver{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Check(context.Background(), safety.Request{SQL: tt.sql})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got.Unsafe != tt.unsafe || len(got.Statements) != tt.wantCount || got.Source != "omni" {
				t.Fatalf("Check() = %+v, want unsafe=%v count=%d source=omni", got, tt.unsafe, tt.wantCount)
			}
			for _, s := range got.Statements {
				if s.Kind != safety.KindUnsafeMissingWhere {
					t.Fatalf("statement kind = %q, want %q", s.Kind, safety.KindUnsafeMissingWhere)
				}
				if s.StartOffset < 0 || s.EndOffset <= s.StartOffset || s.EndOffset > len(tt.sql) {
					t.Fatalf("invalid offsets: %+v (sql len %d)", s, len(tt.sql))
				}
			}
		})
	}
}

func TestPostgresSafetyCheckSyntaxError(t *testing.T) {
	d := &Driver{}
	got, err := d.Check(context.Background(), safety.Request{SQL: "UPDATE widgets SET"})
	if err != nil {
		t.Fatalf("Check: %v, want nil error on syntax error", err)
	}
	if got.Unsafe {
		t.Fatalf("Check() = %+v, want Unsafe=false on syntax error", got)
	}
}
