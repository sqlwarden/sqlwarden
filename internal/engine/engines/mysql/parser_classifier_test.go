package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

func TestMySQLParse(t *testing.T) {
	d := &Driver{}
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

func TestMySQLParseDelimiter(t *testing.T) {
	sql := "DELIMITER //\nCREATE PROCEDURE p() BEGIN SELECT 1; END//\nDELIMITER ;\nSELECT 2;"
	got, err := (&Driver{}).Parse(context.Background(), parser.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.StatementCount != 2 || len(got.Statements) != 2 {
		t.Fatalf("statement count = %d, spans = %+v; want 2", got.StatementCount, got.Statements)
	}
}

func TestMySQLParseExecutableVersionComment(t *testing.T) {
	sql := "SELECT 1 /*!50000; DROP TABLE widgets */"
	got, err := (&Driver{}).Parse(context.Background(), parser.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.StatementCount != 2 || len(got.Statements) != 2 {
		t.Fatalf("statement count = %d, spans = %+v; want two expanded statements", got.StatementCount, got.Statements)
	}
	for _, span := range got.Statements {
		if span != (parser.Statement{StartOffset: 0, EndOffset: len(sql)}) {
			t.Fatalf("expanded statement span = %+v, want shared source range [0,%d)", span, len(sql))
		}
	}
}

func TestMySQLParseSyntaxError(t *testing.T) {
	_, err := (&Driver{}).Parse(context.Background(), parser.Request{SQL: "SELECT é\nFROM"})
	var syntaxErr *parser.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("Parse error = %v, want *parser.SyntaxError", err)
	}
	if syntaxErr.Offset < 0 || syntaxErr.Line < 1 || syntaxErr.Column < 1 {
		t.Fatalf("invalid syntax error position: %+v", syntaxErr)
	}
}

func TestMySQLParseCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&Driver{}).Parse(ctx, parser.Request{SQL: "SELECT 1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse error = %v, want context.Canceled", err)
	}
}

func TestMySQLClassify(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		want      classifier.Kind
		wantCount int
	}{
		{name: "select", sql: "SELECT `id` FROM `widgets`", want: classifier.KindDQL, wantCount: 1},
		{name: "read CTE", sql: "WITH w AS (SELECT 1) SELECT * FROM w", want: classifier.KindDQL, wantCount: 1},
		{name: "CTE update", sql: "WITH active AS (SELECT id FROM widgets WHERE active = true) UPDATE widgets SET active = false WHERE id IN (SELECT id FROM active)", want: classifier.KindDML, wantCount: 1},
		{name: "invalid modifying CTE body", sql: "WITH changed AS (DELETE FROM widgets RETURNING id) SELECT * FROM changed", want: classifier.KindUnknown, wantCount: 0},
		{name: "locking select", sql: "SELECT * FROM widgets FOR UPDATE", want: classifier.KindUnknown, wantCount: 1},
		{name: "nested locking select", sql: "SELECT * FROM (SELECT * FROM widgets FOR UPDATE) AS locked", want: classifier.KindUnknown, wantCount: 1},
		{name: "select into outfile", sql: "SELECT * FROM widgets INTO OUTFILE '/tmp/widgets'", want: classifier.KindUnknown, wantCount: 1},
		{name: "show", sql: "SHOW TABLES", want: classifier.KindDQL, wantCount: 1},
		{name: "describe", sql: "DESCRIBE widgets", want: classifier.KindDQL, wantCount: 1},
		{name: "replace", sql: "REPLACE INTO widgets(id) VALUES (1)", want: classifier.KindDML, wantCount: 1},
		{name: "update", sql: "UPDATE widgets SET active = false", want: classifier.KindDML, wantCount: 1},
		{name: "create", sql: "CREATE TABLE widgets(id bigint)", want: classifier.KindDDL, wantCount: 1},
		{name: "create temporary table", sql: "CREATE TEMPORARY TABLE copied_widgets AS SELECT * FROM widgets", want: classifier.KindDDL, wantCount: 1},
		{name: "load data", sql: "LOAD DATA INFILE '/tmp/widgets.csv' INTO TABLE widgets", want: classifier.KindDML, wantCount: 1},
		{name: "set is unknown", sql: "SET sql_mode = 'STRICT_ALL_TABLES'", want: classifier.KindUnknown, wantCount: 1},
		{name: "transaction is unknown", sql: "START TRANSACTION", want: classifier.KindUnknown, wantCount: 1},
		{name: "privilege statement is unknown", sql: "GRANT SELECT ON app.* TO reader", want: classifier.KindUnknown, wantCount: 1},
		{name: "user administration is unknown", sql: "CREATE USER reader IDENTIFIED BY 'secret'", want: classifier.KindUnknown, wantCount: 1},
		{name: "table lock is unknown", sql: "LOCK TABLES widgets WRITE", want: classifier.KindUnknown, wantCount: 1},
		{name: "procedure call is unknown", sql: "CALL dangerous_proc()", want: classifier.KindUnknown, wantCount: 1},
		{name: "plain explain requires the wrapped statement's permission", sql: "EXPLAIN UPDATE widgets SET active = false", want: classifier.KindDML, wantCount: 1},
		{name: "explain analyze executes", sql: "EXPLAIN ANALYZE UPDATE widgets SET active = false", want: classifier.KindDML, wantCount: 1},
		{name: "read plus DML", sql: "SELECT 1; UPDATE widgets SET active = false", want: classifier.KindDML, wantCount: 2},
		{name: "read plus DDL", sql: "SELECT 1; DROP TABLE widgets", want: classifier.KindDDL, wantCount: 2},
		{name: "mixed mutations", sql: "UPDATE widgets SET active = false; CREATE TABLE audit(id bigint)", want: classifier.KindUnknown, wantCount: 2},
		{name: "versioned comment mutation", sql: "SELECT 1 /*!50000; DROP TABLE widgets */", want: classifier.KindDDL, wantCount: 2},
		{name: "semicolon in string", sql: "SELECT '; DROP TABLE widgets' AS text", want: classifier.KindDQL, wantCount: 1},
		{name: "mutation in comment", sql: "SELECT 1 /* ; DROP TABLE widgets */", want: classifier.KindDQL, wantCount: 1},
		{name: "invalid syntax", sql: "SELECT FROM", want: classifier.KindUnknown, wantCount: 0},
		{name: "empty", sql: "# only a comment", want: classifier.KindUnknown, wantCount: 0},
	}
	d := &Driver{}
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
