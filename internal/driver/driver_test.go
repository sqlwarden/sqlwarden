package driver

import "testing"

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"postgresql": "postgres",
		"sqlite3":    "sqlite",
		"mariadb":    "mysql",
		"postgres":   "postgres",
		"custom":     "custom",
	}

	for input, want := range cases {
		if got := NormalizeName(input); got != want {
			t.Fatalf("NormalizeName(%q) = %q, want %q", input, got, want)
		}
	}
}
