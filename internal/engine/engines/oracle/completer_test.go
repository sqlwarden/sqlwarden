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

func TestOracleCompletionCatalogAndIdentifierInsertion(t *testing.T) {
	scope := oracleSchemaScope("HR")
	table := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "EMP"}
	set := &metadata.MetadataSet{
		Directory: &metadata.Directory{DefaultScope: scope, Roots: []metadata.ScopeNode{{Path: scope, Groups: []metadata.ObjectGroup{
			{Kind: "table", Objects: []metadata.ObjectRef{table, {Scope: scope, Kind: "table", Name: "UNINSPECTED"}}},
			{Kind: "function", Objects: []metadata.ObjectRef{{Scope: scope, Kind: "function", Name: "CALCULATE_TAX"}}},
			{Kind: "sequence", Objects: []metadata.ObjectRef{{Scope: scope, Kind: "sequence", Name: "ORDER_SEQ"}}},
		}}}},
		Objects: []metadata.Object{{Ref: table, Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "Job No"}, {Name: "account.Total"}}}}},
	}
	for _, tt := range []struct{ sql, label, insert string }{
		{"SELECT * FROM UNI|", "UNINSPECTED", "UNINSPECTED"},
		{"SELECT HR.CALC| FROM EMP", "CALCULATE_TAX", "CALCULATE_TAX"},
		{"SELECT ORDER_SEQ.| FROM EMP", "NEXTVAL", "NEXTVAL"},
		{"SELECT e.\"Job |No\" FROM EMP e", "Job No", `"Job No"`},
		{"SELECT e.\"account.|Total\" FROM EMP e", "account.Total", `"account.Total"`},
		{"WITH recent AS (SELECT 1 AS local_id FROM dual) SELECT r.| FROM recent r", "LOCAL_ID", "LOCAL_ID"},
		{"WITH \"Recent Jobs\" AS (SELECT 1 AS \"Job No\" FROM dual) SELECT r.| FROM \"Recent Jobs\" r", "Job No", `"Job No"`},
	} {
		cursor := strings.IndexByte(tt.sql, '|')
		query := strings.Replace(tt.sql, "|", "", 1)
		result, err := (&oracleDriver{}).Complete(context.Background(), completer.Request{SQL: query, CursorOffset: cursor, Schema: set})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, candidate := range result.Suggestions {
			if candidate.Label != tt.label {
				continue
			}
			found = true
			if candidate.InsertText != tt.insert {
				t.Errorf("%s: insert %q, want %q", tt.sql, candidate.InsertText, tt.insert)
			}
			if strings.Contains(tt.sql, "e.\"") {
				applied := query[:candidate.ReplaceStart] + candidate.InsertText + query[candidate.ReplaceEnd:]
				if applied != "SELECT e."+tt.insert+" FROM EMP e" {
					t.Errorf("quoted replacement: %s", applied)
				}
			}
		}
		if !found {
			t.Errorf("%s: missing %s: %+v", tt.sql, tt.label, result.Suggestions)
		}
	}
}
