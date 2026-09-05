package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

func TestSQLiteParseSingleStatement(t *testing.T) {
	d := &sqliteDriver{}
	res, err := d.Parse(context.Background(), parser.Request{SQL: "SELECT 1"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if res.StatementCount != 1 {
		t.Fatalf("StatementCount = %d, want 1", res.StatementCount)
	}
	if len(res.Statements) != 1 {
		t.Fatalf("Statements len = %d, want 1", len(res.Statements))
	}
	if res.Statements[0].StartOffset != 0 || res.Statements[0].EndOffset != len("SELECT 1") {
		t.Fatalf("span = %+v, want {0,8}", res.Statements[0])
	}
}

func TestSQLiteParseMultiStatementSpans(t *testing.T) {
	sql := "SELECT 1; UPDATE t SET a = 1 WHERE id = 2;"
	d := &sqliteDriver{}
	res, err := d.Parse(context.Background(), parser.Request{SQL: sql})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if res.StatementCount != 2 {
		t.Fatalf("StatementCount = %d, want 2", res.StatementCount)
	}
	if got := sql[res.Statements[0].StartOffset:res.Statements[0].EndOffset]; got != "SELECT 1" {
		t.Fatalf("stmt 0 span text = %q, want %q", got, "SELECT 1")
	}
	if got := sql[res.Statements[1].StartOffset:res.Statements[1].EndOffset]; got != "UPDATE t SET a = 1 WHERE id = 2" {
		t.Fatalf("stmt 1 span text = %q", got)
	}
}

func TestSQLiteParseSyntaxError(t *testing.T) {
	d := &sqliteDriver{}
	_, err := d.Parse(context.Background(), parser.Request{SQL: "SELECT FROM WHERE ="})
	var se *parser.SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *parser.SyntaxError", err)
	}
	if se.Line < 1 || se.Column < 1 {
		t.Fatalf("bad position: %+v", se)
	}
}

func TestSQLiteParseContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &sqliteDriver{}
	if _, err := d.Parse(ctx, parser.Request{SQL: "SELECT 1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSQLiteClassify(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want classifier.Kind
	}{
		{"select", "SELECT * FROM t", classifier.KindDQL},
		{"select cte", "WITH x AS (SELECT 1) SELECT * FROM x", classifier.KindDQL},
		{"values", "VALUES (1), (2)", classifier.KindDQL},
		{"insert", "INSERT INTO t (a) VALUES (1)", classifier.KindDML},
		{"insert select", "INSERT INTO t SELECT * FROM u", classifier.KindDML},
		{"update", "UPDATE t SET a = 1", classifier.KindDML},
		{"delete", "DELETE FROM t", classifier.KindDML},
		{"create table", "CREATE TABLE t (a INT)", classifier.KindDDL},
		{"create index", "CREATE INDEX i ON t (a)", classifier.KindDDL},
		{"drop view", "DROP VIEW v", classifier.KindDDL},
		{"alter table", "ALTER TABLE t RENAME TO u", classifier.KindDDL},
		{"pragma", "PRAGMA journal_mode = WAL", classifier.KindUnknown},
		{"attach", "ATTACH DATABASE 'x.db' AS x", classifier.KindUnknown},
		{"cte with delete", "WITH x AS (DELETE FROM t RETURNING *) SELECT * FROM x", classifier.KindUnknown},
		{"explain select", "EXPLAIN SELECT * FROM t", classifier.KindDQL},
		{"mixed dml ddl", "UPDATE t SET a = 1; DROP TABLE t", classifier.KindUnknown},
		{"multi dml", "INSERT INTO t VALUES (1); DELETE FROM t WHERE a = 2", classifier.KindDML},
		{"syntax error", "SELECT FROM WHERE", classifier.KindUnknown},
	}
	d := &sqliteDriver{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := d.Classify(context.Background(), classifier.Request{SQL: tc.sql})
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if res.Kind != tc.want {
				t.Fatalf("Kind = %q, want %q", res.Kind, tc.want)
			}
			if res.Source != "rqlite" {
				t.Fatalf("Source = %q, want rqlite", res.Source)
			}
		})
	}
}
