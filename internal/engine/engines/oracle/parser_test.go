package oracle

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/parser"
)

func TestOracleParseStatementSpans(t *testing.T) {
	d := &oracleDriver{}
	const sql = "SELECT 1 FROM dual;\nSELECT 2 FROM dual"
	res, err := d.Parse(context.Background(), parser.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if res.StatementCount != 2 || len(res.Statements) != 2 {
		t.Fatalf("want 2 statements, got count=%d spans=%d", res.StatementCount, len(res.Statements))
	}
	if got := sql[res.Statements[0].StartOffset:res.Statements[0].EndOffset]; got != "SELECT 1 FROM dual" {
		t.Errorf("span 0 = %q", got)
	}
	if got := sql[res.Statements[1].StartOffset:res.Statements[1].EndOffset]; got != "SELECT 2 FROM dual" {
		t.Errorf("span 1 = %q", got)
	}
}

func TestOracleParseSyntaxError(t *testing.T) {
	d := &oracleDriver{}
	_, err := d.Parse(context.Background(), parser.Request{SQL: "SELECT FROM WHERE ("})
	var se *parser.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("want *parser.SyntaxError, got %v", err)
	}
	if se.Offset < 0 || se.Line < 1 || se.Column < 1 {
		t.Errorf("bad position: %+v", se)
	}
}

func TestOracleParseCancelledContext(t *testing.T) {
	d := &oracleDriver{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Parse(ctx, parser.Request{SQL: "SELECT 1 FROM dual"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
