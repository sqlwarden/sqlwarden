package sqlserver

import (
	"context"
	"testing"
)

func TestParseSQLServerBasic(t *testing.T) {
	tree, spans, err := parseSQLServer(context.Background(), "SELECT 1; SELECT 2;")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || len(spans) != 2 {
		t.Fatalf("want 2 statements, got tree=%d spans=%d", len(tree), len(spans))
	}
}

func TestParseSQLServerGoBatchExcluded(t *testing.T) {
	tree, spans, err := parseSQLServer(context.Background(), "SELECT 1\nGO\nSELECT 2\nGO")
	if err != nil {
		t.Fatal(err)
	}
	// GO is a client-tool batch separator, not executable T-SQL: it must not
	// appear as a span sent to the server.
	if len(tree) != len(spans) {
		t.Fatalf("tree/spans length mismatch: %d vs %d", len(tree), len(spans))
	}
	for _, s := range spans {
		if s.StartOffset == s.EndOffset {
			t.Fatalf("unexpected empty span: %+v", s)
		}
	}
}

func TestParseSQLServerSyntaxError(t *testing.T) {
	_, _, err := parseSQLServer(context.Background(), "SELEC 1")
	if err == nil {
		t.Fatal("want a syntax error")
	}
}
