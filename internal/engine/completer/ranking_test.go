package completer

import "testing"

func TestMatchTier(t *testing.T) {
	tests := []struct {
		name   string
		label  string
		prefix string
		want   int
	}{
		{name: "empty", label: "SUM", prefix: "", want: 0},
		{name: "exact", label: "SUM", prefix: "sum", want: 5},
		{name: "prefix", label: "substring", prefix: "sub", want: 4},
		{name: "segment", label: "array_append_support", prefix: "support", want: 3},
		{name: "substring", label: "consumer", prefix: "sum", want: 2},
		{name: "fuzzy", label: "set_config", prefix: "scf", want: 1},
		{name: "short fuzzy disabled", label: "sequence", prefix: "sq", want: 0},
		{name: "no match", label: "COUNT", prefix: "sum", want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := MatchTier(test.label, test.prefix); got != test.want {
				t.Fatalf("MatchTier(%q, %q) = %d, want %d", test.label, test.prefix, got, test.want)
			}
		})
	}
}

func TestKindScoreOrdersRelationsBeforeOtherObjects(t *testing.T) {
	order := []string{"table", "view", "materialized_view", "foreign_table", "synonym", "schema", "sequence", "function", "procedure"}
	for i := 1; i < len(order); i++ {
		if KindScore(order[i-1]) <= KindScore(order[i]) {
			t.Fatalf("KindScore(%q) = %d, want above KindScore(%q) = %d", order[i-1], KindScore(order[i-1]), order[i], KindScore(order[i]))
		}
	}
	if KindScore("column") <= KindScore("table") {
		t.Fatal("columns must outrank relations")
	}
	if KindScore("not-a-kind") >= KindScore("keyword") {
		t.Fatal("unknown kinds must rank below keywords")
	}
}
