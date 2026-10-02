package mssql

import "testing"

func TestCursorWord(t *testing.T) {
	for _, tc := range []struct {
		sql        string
		cursor     int
		start      int
		prefix     string
		suppressed bool
	}{
		{"SELECT * FROM ord", 17, 14, "ord", false},
		{"SELECT * FROM [ord", 18, 14, "ord", false},
		{"SELECT * FROM ", 14, 14, "", false},
		{"SELECT 'abc", 11, 8, "abc", true},
		{"SELECT 'it''s' , ab", 19, 17, "ab", false},
		{"SELECT 1 -- note ab", 19, 17, "ab", true},
		{"SELECT 1 -- note\nFROM ab", 24, 22, "ab", false},
		{"SELECT /* ab", 12, 10, "ab", true},
		{"SELECT /* x */ ab", 17, 15, "ab", false},
		{"SELECT [a--b] , ab", 18, 16, "ab", false},
		{"SELECT * FROM [order d", 22, 14, "order d", false},
		{"SELECT * FROM [a]]b", 19, 14, "a]b", false},
		{"SELECT * FROM \"order d", 22, 14, "order d", false},
		{"SELECT * FROM [done] x", 22, 21, "x", false},
		{"SELECT pay$ra", 13, 7, "pay$ra", false},
		{"SELECT '[x", 10, 8, "x", true},
	} {
		start, end, prefix, suppressed := CursorWord(tc.sql, tc.cursor)
		if start != tc.start || end != tc.cursor || prefix != tc.prefix || suppressed != tc.suppressed {
			t.Errorf("CursorWord(%q, %d) = %d %d %q %v, want %d %d %q %v",
				tc.sql, tc.cursor, start, end, prefix, suppressed, tc.start, tc.cursor, tc.prefix, tc.suppressed)
		}
	}
}

func TestQuoteIdentifier(t *testing.T) {
	for name, want := range map[string]string{
		"orders":        "orders",
		"status":        "status",
		"order":         "[order]",
		"user":          "[user]",
		"order details": "[order details]",
		"a]b":           "[a]]b]",
		"1st":           "[1st]",
		"":              "[]",
	} {
		if got := QuoteIdentifier(name); got != want {
			t.Errorf("QuoteIdentifier(%q) = %q, want %q", name, got, want)
		}
	}
}
