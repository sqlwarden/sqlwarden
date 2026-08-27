package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

func TestPostgresParse(t *testing.T) {
	d := &postgresDriver{}
	sql := "  SELECT 'é';\nUPDATE widgets SET active = false"
	got, err := d.Parse(context.Background(), parser.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.AST == nil || got.StatementCount != 2 || len(got.Statements) != 2 {
		t.Fatalf("unexpected parse result: %+v", got)
	}
	semicolon := len("  SELECT 'é'")
	if got.Statements[0] != (parser.Statement{StartOffset: 0, EndOffset: semicolon}) {
		t.Errorf("first span = %+v, want [0,%d)", got.Statements[0], semicolon)
	}
	if got.Statements[1].StartOffset != semicolon+1 || got.Statements[1].EndOffset != len(sql) {
		t.Errorf("second span = %+v, want [%d,%d)", got.Statements[1], semicolon+1, len(sql))
	}
}

func TestPostgresParseSyntaxError(t *testing.T) {
	d := &postgresDriver{}
	_, err := d.Parse(context.Background(), parser.Request{SQL: "SELECT é\nFROM"})
	var syntaxErr *parser.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Parse error = %v, want *parser.SyntaxError", err)
	}
	if syntaxErr.Offset < 0 || syntaxErr.Line < 1 || syntaxErr.Column < 1 {
		t.Fatalf("invalid syntax error position: %+v", syntaxErr)
	}
}

func TestPostgresParseCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&postgresDriver{}).Parse(ctx, parser.Request{SQL: "SELECT 1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse error = %v, want context.Canceled", err)
	}
}

func TestPostgresClassify(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		want      classifier.Kind
		wantCount int
	}{
		{name: "select", sql: "SELECT DISTINCT ON (id) id FROM widgets", want: classifier.KindDQL, wantCount: 1},
		{name: "read CTE", sql: "WITH w AS (SELECT 1) SELECT * FROM w", want: classifier.KindDQL, wantCount: 1},
		{name: "modifying CTE", sql: "WITH changed AS (DELETE FROM widgets RETURNING id) SELECT * FROM changed", want: classifier.KindDML, wantCount: 1},
		{name: "mixed DML and DDL CTE query", sql: "WITH changed AS (DELETE FROM widgets RETURNING id) SELECT * INTO archived_widgets FROM changed", want: classifier.KindUnknown, wantCount: 1},
		{name: "locking select", sql: "SELECT * FROM widgets FOR UPDATE", want: classifier.KindUnknown, wantCount: 1},
		{name: "nested locking select", sql: "SELECT * FROM (SELECT * FROM widgets FOR UPDATE) AS locked", want: classifier.KindUnknown, wantCount: 1},
		{name: "select into", sql: "SELECT * INTO archived_widgets FROM widgets", want: classifier.KindDDL, wantCount: 1},
		{name: "insert", sql: "INSERT INTO widgets(id) VALUES (1)", want: classifier.KindDML, wantCount: 1},
		{name: "create", sql: "CREATE TABLE widgets(id bigint)", want: classifier.KindDDL, wantCount: 1},
		{name: "show", sql: "SHOW search_path", want: classifier.KindDQL, wantCount: 1},
		{name: "set is unknown", sql: "SET search_path = public", want: classifier.KindUnknown, wantCount: 1},
		{name: "privilege statement is unknown", sql: "GRANT SELECT ON widgets TO reader", want: classifier.KindUnknown, wantCount: 1},
		{name: "role administration is unknown", sql: "CREATE ROLE reader", want: classifier.KindUnknown, wantCount: 1},
		{name: "plain explain requires the wrapped statement's permission", sql: "EXPLAIN DELETE FROM widgets", want: classifier.KindDML, wantCount: 1},
		{name: "explain analyze executes", sql: "EXPLAIN ANALYZE DELETE FROM widgets", want: classifier.KindDML, wantCount: 1},
		{name: "read plus DML", sql: "SELECT 1; UPDATE widgets SET active = false", want: classifier.KindDML, wantCount: 2},
		{name: "read plus DDL", sql: "SELECT 1; DROP TABLE widgets", want: classifier.KindDDL, wantCount: 2},
		{name: "mixed mutations", sql: "UPDATE widgets SET active = false; CREATE TABLE audit(id bigint)", want: classifier.KindUnknown, wantCount: 2},
		{name: "transaction wrapped mutation", sql: "BEGIN; UPDATE widgets SET active = false; COMMIT", want: classifier.KindUnknown, wantCount: 3},
		{name: "copy program", sql: "COPY widgets TO PROGRAM 'cat > /tmp/widgets.txt'", want: classifier.KindDML, wantCount: 1},
		{name: "procedure call", sql: "CALL dangerous_proc()", want: classifier.KindUnknown, wantCount: 1},
		{name: "do block", sql: "DO $$ BEGIN DELETE FROM widgets; END $$", want: classifier.KindUnknown, wantCount: 1},
		{name: "notification", sql: "NOTIFY widgets_changed, '1'", want: classifier.KindUnknown, wantCount: 1},
		{name: "administrative statement", sql: "VACUUM widgets", want: classifier.KindUnknown, wantCount: 1},
		{name: "semicolon in string", sql: "SELECT '; DELETE FROM widgets' AS text", want: classifier.KindDQL, wantCount: 1},
		{name: "mutation in block comment", sql: "SELECT 1 /* ; DROP TABLE widgets */", want: classifier.KindDQL, wantCount: 1},
		{name: "mutation in line comment", sql: "SELECT 1 -- ; DROP TABLE widgets", want: classifier.KindDQL, wantCount: 1},
		{name: "invalid syntax", sql: "SELECT FROM", want: classifier.KindUnknown, wantCount: 0},
		{name: "empty", sql: "-- only a comment", want: classifier.KindUnknown, wantCount: 0},
	}
	d := &postgresDriver{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Classify(context.Background(), classifier.Request{SQL: tt.sql})
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if got.Kind != tt.want || got.StatementCount != tt.wantCount || got.Source != "omni" {
				t.Fatalf("Classify() = %+v, want kind=%s count=%d source=omni", got, tt.want, tt.wantCount)
			}
		})
	}
}
