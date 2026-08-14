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
