package parser

import "testing"

func TestPositionUsesByteColumnsAndClamps(t *testing.T) {
	tests := []struct {
		name       string
		sql        string
		offset     int
		wantLine   int
		wantColumn int
	}{
		{name: "start", sql: "SELECT", offset: 0, wantLine: 1, wantColumn: 1},
		{name: "unicode bytes", sql: "éx", offset: len("é"), wantLine: 1, wantColumn: 3},
		{name: "next line", sql: "é\nx", offset: len("é\n"), wantLine: 2, wantColumn: 1},
		{name: "negative", sql: "x", offset: -10, wantLine: 1, wantColumn: 1},
		{name: "past end", sql: "x", offset: 10, wantLine: 1, wantColumn: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, column := Position(tt.sql, tt.offset)
			if line != tt.wantLine || column != tt.wantColumn {
				t.Fatalf("Position() = %d:%d, want %d:%d", line, column, tt.wantLine, tt.wantColumn)
			}
		})
	}
}
