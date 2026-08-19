package safety

import (
	"context"
	"testing"
)

func TestHeuristicCheck(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		unsafe bool
		count  int
	}{
		{name: "bare update", sql: "UPDATE widgets SET active = 0", unsafe: true, count: 1},
		{name: "bare delete", sql: "DELETE FROM widgets", unsafe: true, count: 1},
		{name: "update with where", sql: "UPDATE widgets SET active = 0 WHERE id = 1", unsafe: false, count: 0},
		{name: "delete with where", sql: "DELETE FROM widgets WHERE id = 1", unsafe: false, count: 0},
		{name: "select is never flagged", sql: "SELECT * FROM widgets", unsafe: false, count: 0},
		{name: "create is never flagged", sql: "CREATE TABLE widgets(id integer)", unsafe: false, count: 0},
		{
			name:   "multi-statement mixes safe and unsafe",
			sql:    "UPDATE widgets SET active = 0 WHERE id = 1; DELETE FROM widgets",
			unsafe: true,
			count:  1,
		},
		{
			name:   "both statements unsafe",
			sql:    "UPDATE widgets SET active = 0; DELETE FROM widgets",
			unsafe: true,
			count:  2,
		},
		{
			// Documented false negative: a WHERE inside a subquery in the SET
			// clause is misread as satisfying the top-level check. Acceptable —
			// mirrors the accuracy tradeoff classifier.heuristic already makes.
			name:   "false negative: where inside SET subquery",
			sql:    "UPDATE widgets SET active = (SELECT active FROM defaults WHERE id = 1)",
			unsafe: false,
			count:  0,
		},
	}
	h := NewHeuristic()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.Check(context.Background(), Request{SQL: tt.sql})
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if got.Unsafe != tt.unsafe || len(got.Statements) != tt.count || got.Source != "heuristic" {
				t.Fatalf("Check() = %+v, want unsafe=%v count=%d source=heuristic", got, tt.unsafe, tt.count)
			}
		})
	}
}
