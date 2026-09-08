package sqlserver

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
)

func TestGenerateInsertUsesNamedParamsAndBrackets(t *testing.T) {
	d := &Driver{}
	req := statement.Request{
		Operation: statement.OperationInsert,
		Object: metadata.Object{
			Ref: metadata.ObjectRef{
				Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
				Kind:  "table",
				Name:  "widgets",
			},
			Relational: &metadata.RelationalDetail{
				Columns: []metadata.Column{{Name: "id"}, {Name: "label"}},
			},
		},
	}
	sql, err := d.Generate(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "[dbo].[widgets]") {
		t.Fatalf("want bracket-quoted qualified name, got %q", sql)
	}
	if !strings.Contains(sql, "@p1") || !strings.Contains(sql, "@p2") {
		t.Fatalf("want named parameters @p1/@p2, got %q", sql)
	}
	if strings.Contains(sql, "?") {
		t.Fatalf("must not contain ? placeholders (invalid for go-mssqldb), got %q", sql)
	}
}
