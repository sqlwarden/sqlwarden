package classifier

import (
	"context"
	"testing"
)

func TestHeuristicClassifies(t *testing.T) {
	c := NewHeuristic()
	cases := []struct {
		sql  string
		want Kind
	}{
		{"DROP TABLE t", KindDDL},
		{"DELETE FROM t WHERE id = 1", KindDML},
		{"SELECT * FROM t", KindDQL},
		{"VACUUM", KindUnknown},
	}
	for _, tc := range cases {
		got, err := c.Classify(context.Background(), Request{SQL: tc.sql})
		if err != nil {
			t.Fatalf("Classify(%q): %v", tc.sql, err)
		}
		if got.Kind != tc.want {
			t.Errorf("Classify(%q) = %q, want %q", tc.sql, got.Kind, tc.want)
		}
		if got.Source != "heuristic" {
			t.Errorf("Classify(%q) source = %q, want heuristic", tc.sql, got.Source)
		}
	}
}
