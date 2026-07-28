package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine/completer"
	"github.com/sqlwarden/internal/dbengine/schema"
)

func TestPostgresCompleteKeywordsAndSchema(t *testing.T) {
	driver := &postgresDriver{}
	keywordResult, err := driver.Complete(context.Background(), completer.Request{
		SQL: "SEL", CursorOffset: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, keywordResult, "SELECT", "keyword")

	catalog := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")
	sql := "SELECT  FROM public.users"
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len("SELECT "), Catalog: catalog, Objects: objects,
		ConnectionID: "7", CatalogVersion: "snapshot-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "display name", "column")

	fromSQL := "SELECT * FROM "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: fromSQL, CursorOffset: len(fromSQL), Catalog: catalog, Objects: objects,
	})
	if err != nil {
		t.Fatal(err)
	}
	suggestion := requireCompletion(t, result, "Order Items", "table")
	if suggestion.InsertText != `"Order Items"` {
		t.Fatalf("quoted insert text = %q", suggestion.InsertText)
	}
}

func TestPostgresCompleteRejectsInvalidCursorAndCancellation(t *testing.T) {
	driver := &postgresDriver{}
	if _, err := driver.Complete(context.Background(), completer.Request{SQL: "x", CursorOffset: 2}); err == nil {
		t.Fatal("expected invalid cursor error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := driver.Complete(ctx, completer.Request{}); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestPostgresCompletionQuotesReservedIdentifier(t *testing.T) {
	if got := postgresQuoteCompletionIdentifier("order"); got != `"order"` {
		t.Fatalf("reserved identifier insertion = %q", got)
	}
}

func TestPostgresCompleteRespectsQualifiedAliasesAndJoinConflicts(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "inventory"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "bigint"}, {Name: "inventory_name", DataType: "text"},
			}},
		},
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "store"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "bigint"}, {Name: "store_name", DataType: "text"},
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
	requireCompletion(t, qualified, "store_name", "column")
	requireNoCompletion(t, qualified, "inventory_name", "column")

	unqualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE "
	unqualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: unqualifiedSQL, CursorOffset: len(unqualifiedSQL), Catalog: catalog, Objects: objects,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, unqualified, "i.id", "column")
	requireCompletion(t, unqualified, "s.id", "column")
	requireCompletion(t, unqualified, "inventory_name", "column")
	requireCompletion(t, unqualified, "store_name", "column")
}

func TestPostgresCompleteUsesFinalAliasAfterEarlierQualifiedColumn(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "film"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "film_id", DataType: "smallint"},
				{Name: "description", DataType: "text"},
			}},
		},
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "film_actor"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "actor_id", DataType: "smallint"},
				{Name: "film_id", DataType: "smallint"},
			}},
		},
	}
	catalog.Namespaces[0].Groups[0].Objects = []schema.ObjectRef{objects[0].Ref, objects[1].Ref}

	sql := "select * from film f\njoin film_actor fa\nwhere f.\"description\" = fa."
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql), Catalog: catalog, Objects: objects,
		TriggerKind: completer.TriggerAutomatic, TriggerChar: ".",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "actor_id", "column")
	requireCompletion(t, result, "film_id", "column")
	requireNoCompletion(t, result, "description", "column")
}

func TestPostgresAutomaticBareSelectIsCurated(t *testing.T) {
	driver := &postgresDriver{}
	sql := "SELECT "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql), TriggerKind: completer.TriggerAutomatic,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "COUNT", "function")
	requireCompletion(t, result, "DISTINCT", "keyword")
	if len(result.Suggestions) != 10 {
		t.Fatalf("curated suggestions = %d, want 10: %+v", len(result.Suggestions), result.Suggestions)
	}
	requireNoCompletion(t, result, "ALTER", "keyword")
}

func TestPostgresCompletionVocabulary(t *testing.T) {
	vocabulary := (&postgresDriver{}).CompletionVocabulary()
	if vocabulary.Dialect != "postgres" || vocabulary.Version == "" {
		t.Fatalf("invalid vocabulary metadata: %+v", vocabulary)
	}
	requireCompletion(t, completer.Result{Suggestions: vocabulary.Suggestions}, "SELECT", "keyword")
	requireCompletion(t, completer.Result{Suggestions: vocabulary.Suggestions}, "count", "function")
}

func completionTestCatalog(dialect, namespace string) *schema.Catalog {
	return &schema.Catalog{
		Dialect: dialect, Database: namespace,
		Namespaces: []schema.NamespaceCatalog{{
			Name: namespace,
			Groups: []schema.ObjectGroupCatalog{{
				Kind: "table",
				Objects: []schema.ObjectRef{
					{Namespace: namespace, Kind: "table", Name: "users"},
					{Namespace: namespace, Kind: "table", Name: "Order Items"},
				},
			}},
		}},
	}
}

func completionTestObjects(namespace string) []schema.Object {
	return []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: namespace, Kind: "table", Name: "users"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "bigint"},
				{Name: "display name", DataType: "unsupported;type"},
			}},
		},
		{
			Ref:        schema.ObjectRef{Namespace: namespace, Kind: "table", Name: "Order Items"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{{Name: "id", DataType: "bigint"}}},
		},
	}
}

func requireCompletion(t *testing.T, result completer.Result, label, kind string) completer.Suggestion {
	t.Helper()
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == label && suggestion.Kind == kind {
			return suggestion
		}
	}
	t.Fatalf("completion %q (%s) missing from %+v", label, kind, result.Suggestions)
	return completer.Suggestion{}
}

func requireNoCompletion(t *testing.T, result completer.Result, label, kind string) {
	t.Helper()
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == label && suggestion.Kind == kind {
			t.Fatalf("unexpected completion %q (%s) in %+v", label, kind, result.Suggestions)
		}
	}
}
