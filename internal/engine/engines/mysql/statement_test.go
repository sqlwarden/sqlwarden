package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
)

func mysqlStatementObject(kind string) metadata.Object {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	return metadata.Object{
		Ref: metadata.ObjectRef{Scope: scope, Kind: kind, Name: "generated`orders"},
		Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
			{Name: "id", DataType: "int", Ordinal: 1},
			{Name: "order`note", DataType: "text", Ordinal: 2},
		}},
	}
}

func TestMySQLGenerateStatement(t *testing.T) {
	driver := &Driver{}
	object := mysqlStatementObject("table")
	generated, err := driver.Generate(statement.Request{Operation: statement.OperationInsert, Object: object})
	if err != nil {
		t.Fatal(err)
	}
	want := "INSERT INTO `testdb`.`generated``orders` (\n  `id`,\n  `order``note`\n)\nVALUES (\n  ?,\n  ?\n);"
	if generated != want {
		t.Fatalf("generated INSERT:\n%s\nwant:\n%s", generated, want)
	}

	view := object
	view.Ref.Kind = "view"
	if _, err := driver.Generate(statement.Request{Operation: statement.OperationDelete, Object: view}); err == nil {
		t.Fatal("expected DELETE generation for a view to fail")
	}
}

func TestMySQLGeneratedStatementsExecuteSafely(t *testing.T) {
	driver := newConnectedDriver(t)
	ctx := context.Background()
	object := mysqlStatementObject("table")
	qualified := mysqlQuoteQualified("testdb", object.Ref.Name)
	if _, err := driver.db.ExecContext(ctx, "CREATE TABLE "+qualified+" (id int, `order``note` text)"); err != nil {
		t.Fatal(err)
	}

	insertSQL, err := driver.Generate(statement.Request{Operation: statement.OperationInsert, Object: object})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.db.ExecContext(ctx, insertSQL); err == nil {
		t.Fatal("unfilled INSERT placeholders unexpectedly executed")
	}
	if _, err := driver.db.ExecContext(ctx, insertSQL, 1, "kept"); err != nil {
		t.Fatalf("filled INSERT: %v", err)
	}

	for _, operation := range []statement.Operation{statement.OperationUpdate, statement.OperationDelete} {
		generated, err := driver.Generate(statement.Request{Operation: operation, Object: object})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(generated, "WHERE 1 = 0") {
			t.Fatalf("%s lacks safe predicate: %s", operation, generated)
		}
		args := []any(nil)
		if operation == statement.OperationUpdate {
			args = []any{2, "changed"}
		}
		if _, err := driver.db.ExecContext(ctx, generated, args...); err != nil {
			t.Fatalf("execute %s: %v", operation, err)
		}
	}

	var id int
	var note string
	if err := driver.db.QueryRowContext(ctx, "SELECT id, `order``note` FROM "+qualified).Scan(&id, &note); err != nil {
		t.Fatal(err)
	}
	if id != 1 || note != "kept" {
		t.Fatalf("safe templates changed the row: id=%d note=%q", id, note)
	}
}
