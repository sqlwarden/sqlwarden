package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestOracleCompleteKeywords(t *testing.T) {
	d := &oracleDriver{}
	res, err := d.Complete(context.Background(), completer.Request{SQL: "SEL", CursorOffset: 3})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	found := false
	for _, s := range res.Suggestions {
		if s.Label == "SELECT" && s.Kind == "keyword" {
			found = true
		}
		if s.ReplaceStart != 0 || s.ReplaceEnd != 3 {
			t.Errorf("replace span = [%d,%d], want [0,3]", s.ReplaceStart, s.ReplaceEnd)
		}
	}
	if !found {
		t.Fatalf("SELECT keyword missing from %v", res.Suggestions)
	}
}

func TestOracleCompleteCursorOutOfRange(t *testing.T) {
	d := &oracleDriver{}
	if _, err := d.Complete(context.Background(), completer.Request{SQL: "SELECT", CursorOffset: 99}); err == nil {
		t.Fatal("expected out-of-range error")
	}
}

func TestOracleCompletionVocabulary(t *testing.T) {
	v := (&oracleDriver{}).CompletionVocabulary()
	if v.Dialect != "oracle" {
		t.Fatalf("dialect = %q", v.Dialect)
	}
	if v.Version == "" {
		t.Fatal("vocabulary needs a deterministic version")
	}
	joined := ""
	for _, s := range v.Suggestions {
		joined += " " + s.Label
	}
	if !strings.Contains(strings.ToUpper(joined), "SELECT") {
		t.Fatalf("vocabulary missing SELECT: %q", joined)
	}
}

func TestOracleInvalidateCompletionCatalogNoPanic(t *testing.T) {
	(&oracleDriver{}).InvalidateCompletionCatalog("conn-1")
}

func TestOracleCompleteColumnsFromSchema(t *testing.T) {
	d := &oracleDriver{}
	set := &metadata.MetadataSet{
		Version: "v1",
		Directory: &metadata.Directory{
			Engine:       "oracle",
			DefaultScope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "HR"}),
			Roots: []metadata.ScopeNode{{
				Path: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "HR"}),
				Groups: []metadata.ObjectGroup{{
					Kind: "table",
					Objects: []metadata.ObjectRef{{
						Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "HR"}),
						Kind:  "table", Name: "EMPLOYEES",
					}},
				}},
			}},
		},
		Objects: []metadata.Object{{
			Ref: metadata.ObjectRef{
				Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "HR"}),
				Kind:  "table", Name: "EMPLOYEES",
			},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "EMPLOYEE_ID"}, {Name: "FIRST_NAME"}}},
		}},
	}
	const sql = "SELECT  FROM EMPLOYEES"
	res, err := d.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len("SELECT "), Schema: set, ConnectionID: "conn-1",
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var cols []string
	for _, s := range res.Suggestions {
		if s.Kind == "column" {
			cols = append(cols, s.Label)
		}
	}
	if len(cols) < 2 {
		t.Fatalf("expected EMPLOYEES columns, got suggestions %+v", res.Suggestions)
	}
}
