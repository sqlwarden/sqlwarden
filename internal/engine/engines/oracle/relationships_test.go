package oracle

import (
	"strings"
	"testing"
)

func TestOracleRelationshipsQueryShape(t *testing.T) {
	q := oracleRelationshipsQuery
	for _, want := range []string{
		"all_constraints", "constraint_type = 'R'", "c.owner = :1",
		"r_constraint_name", "ORDER BY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("relationships query missing %q", want)
		}
	}
}
