package mysql

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/schema"
)

func TestMySQLCompleteKeywordsAndSchema(t *testing.T) {
	driver := &mysqlDriver{}
	keywordResult, err := driver.Complete(context.Background(), completer.Request{
		SQL: "SEL", CursorOffset: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, keywordResult, "SELECT", "keyword")

	catalog := mysqlCompletionTestCatalog()
	objects := mysqlCompletionTestObjects()
	sql := "SELECT  FROM users"
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len("SELECT "), Catalog: catalog, Objects: objects,
		ConnectionID: "8", CatalogVersion: "snapshot-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "display name", "column")

	fromSQL := "SELECT * FROM "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: fromSQL, CursorOffset: len(fromSQL), Catalog: catalog, Objects: objects,
	})
	if err != nil {
		t.Fatal(err)
	}
	suggestion := requireMySQLCompletion(t, result, "Order Items", "table")
	if suggestion.InsertText != "`Order Items`" {
		t.Fatalf("quoted insert text = %q", suggestion.InsertText)
	}
}

func TestMySQLCompleteRejectsInvalidCursorAndCancellation(t *testing.T) {
	driver := &mysqlDriver{}
	if _, err := driver.Complete(context.Background(), completer.Request{SQL: "x", CursorOffset: 2}); err == nil {
		t.Fatal("expected invalid cursor error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := driver.Complete(ctx, completer.Request{}); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestMySQLCompletionQuotesReservedIdentifier(t *testing.T) {
	if got := mysqlQuoteCompletionIdentifier("order"); got != "`order`" {
		t.Fatalf("reserved identifier insertion = %q", got)
	}
}

func TestMySQLCompleteRespectsQualifiedAliasesAndJoinConflicts(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "app", Kind: "table", Name: "inventory"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "BIGINT"}, {Name: "inventory_name", DataType: "TEXT"},
			}},
		},
		{
			Ref: schema.ObjectRef{Namespace: "app", Kind: "table", Name: "store"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "BIGINT"}, {Name: "store_name", DataType: "TEXT"},
			}},
		},
	}
	catalog.Namespaces[0].Groups[0].Objects = []schema.ObjectRef{objects[0].Ref, objects[1].Ref}

	qualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE s."
	qualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: qualifiedSQL, CursorOffset: len(qualifiedSQL), Catalog: catalog, Objects: objects,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, qualified, "store_name", "column")
	requireNoMySQLCompletion(t, qualified, "inventory_name", "column")

	unqualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE "
	unqualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: unqualifiedSQL, CursorOffset: len(unqualifiedSQL), Catalog: catalog, Objects: objects,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, unqualified, "i.id", "column")
	requireMySQLCompletion(t, unqualified, "s.id", "column")
}

func TestMySQLCompleteUsesFinalAliasAfterEarlierQualifiedColumn(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "app", Kind: "table", Name: "film"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "film_id", DataType: "SMALLINT"},
				{Name: "description", DataType: "TEXT"},
			}},
		},
		{
			Ref: schema.ObjectRef{Namespace: "app", Kind: "table", Name: "film_actor"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "actor_id", DataType: "SMALLINT"},
				{Name: "film_id", DataType: "SMALLINT"},
			}},
		},
	}
	catalog.Namespaces[0].Groups[0].Objects = []schema.ObjectRef{objects[0].Ref, objects[1].Ref}

	sql := "select * from film f\njoin film_actor fa\nwhere f.`description` = fa."
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql), Catalog: catalog, Objects: objects,
		TriggerKind: completer.TriggerAutomatic, TriggerChar: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "actor_id", "column")
	requireMySQLCompletion(t, result, "film_id", "column")
	requireNoMySQLCompletion(t, result, "description", "column")
}

func TestMySQLAutomaticBareSelectIsCurated(t *testing.T) {
	driver := &mysqlDriver{}
	sql := "SELECT "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql), TriggerKind: completer.TriggerAutomatic,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "COUNT", "function")
	requireMySQLCompletion(t, result, "DISTINCT", "keyword")
	if len(result.Suggestions) != 10 {
		t.Fatalf("curated suggestions = %d, want 10: %+v", len(result.Suggestions), result.Suggestions)
	}
	requireNoMySQLCompletion(t, result, "ALTER", "keyword")
}

func TestMySQLCompletionVocabulary(t *testing.T) {
	vocabulary := (&mysqlDriver{}).CompletionVocabulary()
	if vocabulary.Dialect != "mysql" || vocabulary.Version == "" {
		t.Fatalf("invalid vocabulary metadata: %+v", vocabulary)
	}
	requireMySQLCompletion(t, completer.Result{Suggestions: vocabulary.Suggestions}, "SELECT", "keyword")
	requireMySQLCompletion(t, completer.Result{Suggestions: vocabulary.Suggestions}, "COUNT", "function")
}

func mysqlCompletionTestCatalog() *schema.Catalog {
	return &schema.Catalog{
		Dialect: "mysql", Database: "app",
		Namespaces: []schema.NamespaceCatalog{{
			Name: "app",
			Groups: []schema.ObjectGroupCatalog{{
				Kind: "table",
				Objects: []schema.ObjectRef{
					{Namespace: "app", Kind: "table", Name: "users"},
					{Namespace: "app", Kind: "table", Name: "Order Items"},
				},
			}},
		}},
	}
}

func mysqlCompletionTestObjects() []schema.Object {
	return []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "app", Kind: "table", Name: "users"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "BIGINT"},
				{Name: "display name", DataType: "unsupported;type"},
			}},
		},
		{
			Ref:        schema.ObjectRef{Namespace: "app", Kind: "table", Name: "Order Items"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{{Name: "id", DataType: "BIGINT"}}},
		},
	}
}

func requireMySQLCompletion(t *testing.T, result completer.Result, label, kind string) completer.Suggestion {
	t.Helper()
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == label && suggestion.Kind == kind {
			return suggestion
		}
	}
	t.Fatalf("completion %q (%s) missing from %+v", label, kind, result.Suggestions)
	return completer.Suggestion{}
}

func requireNoMySQLCompletion(t *testing.T, result completer.Result, label, kind string) {
	t.Helper()
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == label && suggestion.Kind == kind {
			t.Fatalf("unexpected completion %q (%s) in %+v", label, kind, result.Suggestions)
		}
	}
}
