package oracle

import "testing"

func TestOracleQuoteIdent(t *testing.T) {
	cases := map[string]string{
		"EMP":    `"EMP"`,
		"my col": `"my col"`,
		`a"b`:    `"a""b"`,
		"lower":  `"lower"`,
		"":       `""`,
	}
	for in, want := range cases {
		if got := oracleQuoteIdent(in); got != want {
			t.Errorf("oracleQuoteIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOracleQualified(t *testing.T) {
	if got := oracleQualified("HR", "EMPLOYEES"); got != `"HR"."EMPLOYEES"` {
		t.Errorf("got %q", got)
	}
	if got := oracleQualified("", "EMPLOYEES"); got != `"EMPLOYEES"` {
		t.Errorf("got %q", got)
	}
}
