package classifier

import "testing"

func TestCountStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want int
	}{
		{"empty", "", 0},
		{"whitespace only", "   \n\t", 0},
		{"single no terminator", "SELECT 1", 1},
		{"single with terminator", "SELECT 1;", 1},
		{"trailing whitespace after terminator", "SELECT 1;   ", 1},
		{"two statements", "SELECT 1; SELECT 2", 2},
		{"two statements both terminated", "SELECT 1; SELECT 2;", 2},
		{"semicolon inside single-quoted string", "SELECT ';'", 1},
		{"semicolon inside double-quoted identifier", `SELECT "a;b"`, 1},
		{"escaped single quote", "SELECT 'it''s; fine'", 1},
		{"semicolon inside line comment", "SELECT 1 -- comment; with semi\n", 1},
		{"semicolon inside block comment", "SELECT 1 /* comment; with semi */", 1},
		{"statement after block comment", "SELECT 1 /* c */; SELECT 2", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountStatements(tt.sql); got != tt.want {
				t.Errorf("CountStatements(%q) = %d, want %d", tt.sql, got, tt.want)
			}
		})
	}
}
