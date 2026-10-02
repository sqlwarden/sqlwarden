package sqlserver

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSQLServerCompleteKeyword(t *testing.T) {
	driver := &Driver{}
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: "SELE", CursorOffset: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == "SELECT" && suggestion.Kind == "keyword" {
			return
		}
	}
	t.Fatalf("expected SELECT keyword suggestion, got %+v", result.Suggestions)
}

func TestSQLServerTreeFallsBackToDbo(t *testing.T) {
	if got := (&Driver{}).Tree().FallbackScopes; len(got) != 1 || got[0] != "dbo" {
		t.Fatalf("fallback scopes = %v", got)
	}
}

func TestSQLServerCompleteQuotesObjectsAndReportsDemands(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	view := metadata.NewCompletionView(navigatorTree, sales, "", map[metadata.ListingKey][]metadata.Child{
		{Parent: app, Folder: "schemas"}:           {{Kind: "schema", Name: "sales"}},
		{Parent: sales, Folder: "tables"}:          {{Kind: "table", Name: "order"}, {Kind: "table", Name: "order details"}, {Kind: "table", Name: "orders"}},
		{Parent: sales, Folder: "external_tables"}: {{Kind: "external_table", Name: "ext"}},
		{Parent: sales, Folder: "views"}:           {},
		{Parent: sales, Folder: "procedures"}:      {},
		{Parent: sales, Folder: "synonyms"}:        {},
	}, nil)
	sql := "SELECT * FROM [ord"
	result, err := (&Driver{}).Complete(context.Background(), completer.Request{SQL: sql, CursorOffset: len(sql), Metadata: view})
	if err != nil {
		t.Fatal(err)
	}
	inserts := map[string]string{}
	for _, suggestion := range result.Suggestions {
		if suggestion.Kind == "table" {
			inserts[suggestion.Label] = suggestion.InsertText
			if suggestion.ReplaceStart != len("SELECT * FROM ") || suggestion.ReplaceEnd != len(sql) {
				t.Fatalf("%s replaces %d..%d", suggestion.Label, suggestion.ReplaceStart, suggestion.ReplaceEnd)
			}
		}
	}
	want := map[string]string{"order": "[order]", "order details": "[order details]", "orders": "orders"}
	for label, insert := range want {
		if inserts[label] != insert {
			t.Fatalf("insert texts = %v, want %v", inserts, want)
		}
	}
	if result.Context != "relation" {
		t.Fatalf("context = %q", result.Context)
	}
	if len(result.Demands) != 0 {
		t.Fatalf("demands = %v", result.Demands)
	}

	sql = "SELECT * FROM "
	result, err = (&Driver{}).Complete(context.Background(), completer.Request{SQL: sql, CursorOffset: len(sql), Metadata: view})
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == "ext" {
			if suggestion.Kind != "foreign_table" || suggestion.Score != completer.KindScore("foreign_table") {
				t.Fatalf("ext = %+v", suggestion)
			}
			return
		}
	}
	t.Fatalf("external table missing from %+v", result.Suggestions)
}

func TestKindScoreRanksExternalTablesWithForeignTables(t *testing.T) {
	if completer.KindScore("external_table") != completer.KindScore("foreign_table") {
		t.Fatalf("external_table = %d", completer.KindScore("external_table"))
	}
}
