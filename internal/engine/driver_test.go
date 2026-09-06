package engine

import "testing"

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"postgresql": "postgres",
		"sqlite3":    "sqlite",
		// MariaDB is its own registered engine, not an alias of "mysql".
		"mariadb": "mariadb",
		"mysql":   "mysql",
		"oracle":  "oracle",
	}
	for input, want := range cases {
		if got := NormalizeName(input); got != want {
			t.Errorf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}
