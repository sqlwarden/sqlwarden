package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
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
		SQL: sql, CursorOffset: len("SELECT "),
		Schema:       &metadata.MetadataSet{Directory: catalog, Objects: objects, Version: "snapshot-1"},
		ConnectionID: "7",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "display name", "column")

	fromSQL := "SELECT * FROM "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: fromSQL, CursorOffset: len(fromSQL),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
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

func TestPostgresCompletionDefaultSchema(t *testing.T) {
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	tenant := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "tenant"})
	directory := &metadata.Directory{
		DefaultScope: tenant,
		Roots: []metadata.ScopeNode{{Path: root, Children: []metadata.ScopeNode{
			{Path: public}, {Path: tenant},
		}}},
	}
	if got := postgresCompletionDefaultSchema(directory); got != "tenant" {
		t.Fatalf("default schema = %q", got)
	}
	directory.DefaultScope = ""
	if got := postgresCompletionDefaultSchema(directory); got != "public" {
		t.Fatalf("public fallback = %q", got)
	}
	only := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "only_schema"})
	directory.Roots[0].Children = []metadata.ScopeNode{{Path: only}}
	if got := postgresCompletionDefaultSchema(directory); got != "only_schema" {
		t.Fatalf("single-schema fallback = %q", got)
	}
}

func TestPostgresCompleteUsesDirectoryDefaultSchemaForUnqualifiedTables(t *testing.T) {
	driver := &postgresDriver{}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "analytics"})
	public := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	tenant := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "tenant"})
	directory := &metadata.Directory{
		Engine: "postgres", DefaultScope: tenant,
		Roots: []metadata.ScopeNode{{Path: root, Children: []metadata.ScopeNode{
			{
				Path: public,
				Groups: []metadata.ObjectGroup{{
					Kind: "table",
					Objects: []metadata.ObjectRef{{
						Scope: public, Kind: "table", Name: "public_orders",
					}},
				}},
			},
			{
				Path: tenant,
				Groups: []metadata.ObjectGroup{{
					Kind: "table",
					Objects: []metadata.ObjectRef{{
						Scope: tenant, Kind: "table", Name: "tenant_orders",
					}},
				}},
			},
		}}},
	}
	objects := []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: public, Kind: "table", Name: "public_orders"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{
				Name: "id", DataType: "bigint",
			}}},
		},
		{
			Ref: metadata.ObjectRef{Scope: tenant, Kind: "table", Name: "tenant_orders"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{
				Name: "id", DataType: "bigint",
			}}},
		},
	}
	sql := "SELECT * FROM "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: directory, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "tenant_orders", "table")
	requireNoCompletion(t, result, "public_orders", "table")
}

func TestPostgresCompleteHidesNativeParserArtifacts(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")

	afterSelectList := "SELECT * "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: afterSelectList, CursorOffset: len(afterSelectList),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "FROM", "keyword")
	requireNoCompletion(t, result, ",", "keyword")
	requireNoCompletion(t, result, ";", "keyword")

	afterFrom := "SELECT * FROM "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: afterFrom, CursorOffset: len(afterFrom),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "users", "table")
	requireNoCompletion(t, result, "_x", "table")
	requireNoCompletion(t, result, "(", "keyword")
}

func TestPostgresCompleteCuratesCompletedRelationContext(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")

	sql := "SELECT * FROM users "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerInvoked,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"AS", "JOIN", "WHERE", "GROUP", "ORDER", "LIMIT"} {
		requireCompletion(t, result, label, "keyword")
	}
	for _, label := range []string{"ABORT", "ALTER", "DATABASE", "ON", "USING"} {
		requireNoCompletion(t, result, label, "keyword")
	}

	joinedSQL := "SELECT * FROM users u JOIN \"Order Items\" oi "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: joinedSQL, CursorOffset: len(joinedSQL),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerInvoked,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "ON", "keyword")
	requireCompletion(t, result, "USING", "keyword")
	requireNoCompletion(t, result, "AS", "keyword")
	requireNoCompletion(t, result, "ALTER", "keyword")
}

func TestPostgresCompleteUsesStatementAtCursor(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")
	sql := `select s.first_name, s.last_name, a.address from staff s
join store st
on s.staff_id = st.manager_staff_id
join address a
on a.address_id = s.address_id;

select * from `
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "users", "table")
}

func TestPostgresCompleteRecoversNewSelectWithoutSemicolon(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")
	sql := `select s.first_name, s.last_name, a.address from staff s
join store st
on s.staff_id = st.manager_staff_id
join address a
on a.address_id = s.address_id;

select * from actor

select * from `
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "users", "table")
}

func TestPostgresCompletionRecoveryDoesNotSplitNestedSelects(t *testing.T) {
	for _, sql := range []string{
		"WITH recent AS (\n  SELECT * FROM users\n)\nSELECT * FROM ",
		"SELECT * FROM (\n  SELECT * FROM users\n) nested WHERE ",
	} {
		if recovered, _, ok := postgresCompletionRecoveryStatement(sql, len(sql)); ok {
			t.Fatalf("unexpected recovery statement %q for %q", recovered, sql)
		}
	}
}

func TestPostgresCompleteRespectsQualifiedAliasesAndJoinConflicts(t *testing.T) {
	driver := &postgresDriver{}
	catalog := completionTestCatalog("postgres", "public")
	objects := []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: completionTestScope("public"), Kind: "table", Name: "inventory"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "bigint"}, {Name: "inventory_name", DataType: "text"},
			}},
		},
		{
			Ref: metadata.ObjectRef{Scope: completionTestScope("public"), Kind: "table", Name: "store"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "bigint"}, {Name: "store_name", DataType: "text"},
			}},
		},
	}
	catalog.Roots[0].Groups[0].Objects = []metadata.ObjectRef{objects[0].Ref, objects[1].Ref}

	qualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE s."
	qualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: qualifiedSQL, CursorOffset: len(qualifiedSQL),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, qualified, "store_name", "column")
	requireNoCompletion(t, qualified, "inventory_name", "column")

	unqualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE "
	unqualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: unqualifiedSQL, CursorOffset: len(unqualifiedSQL),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
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
	objects := []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: completionTestScope("public"), Kind: "table", Name: "film"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "film_id", DataType: "smallint"},
				{Name: "description", DataType: "text"},
			}},
		},
		{
			Ref: metadata.ObjectRef{Scope: completionTestScope("public"), Kind: "table", Name: "film_actor"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "actor_id", DataType: "smallint"},
				{Name: "film_id", DataType: "smallint"},
			}},
		},
	}
	catalog.Roots[0].Groups[0].Objects = []metadata.ObjectRef{objects[0].Ref, objects[1].Ref}

	sql := "select * from film f\njoin film_actor fa\nwhere f.\"description\" = fa."
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerAutomatic,
		TriggerChar: ".",
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
	requireCompletion(t, completer.Result{Suggestions: vocabulary.Suggestions}, "sum", "function")
}

func TestPostgresSuggestionRankingPrioritizesTypedPrefix(t *testing.T) {
	suggestions := []completer.Suggestion{
		{Label: "consumer", Score: 100},
		{Label: "summary", Score: 1},
		{Label: "gross_sum", Score: 100},
		{Label: "SUM", Score: 1},
	}
	sortSuggestions(suggestions, "sum")
	want := []string{"SUM", "summary", "gross_sum", "consumer"}
	for i, label := range want {
		if suggestions[i].Label != label {
			t.Fatalf("suggestion %d = %q, want %q: %+v", i, suggestions[i].Label, label, suggestions)
		}
	}
}

func TestPostgresCompleteDoesNotEchoUnknownRelationPrefix(t *testing.T) {
	directory := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")
	sql := "SELECT * FROM veraxasdwadqwd"
	result, err := (&postgresDriver{}).Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: directory, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoCompletion(t, result, "veraxasdwadqwd", "table")

	partialSQL := "SELECT * FROM us"
	partial, err := (&postgresDriver{}).Complete(context.Background(), completer.Request{
		SQL: partialSQL, CursorOffset: len(partialSQL),
		Schema: &metadata.MetadataSet{Directory: directory, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, partial, "users", "table")
	requireNoCompletion(t, partial, "us", "table")
}

func TestPostgresCompletePreservesCTERelation(t *testing.T) {
	directory := completionTestCatalog("postgres", "public")
	objects := completionTestObjects("public")
	sql := "WITH recent_orders AS (SELECT * FROM users) SELECT * FROM recent"
	result, err := (&postgresDriver{}).Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: directory, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCompletion(t, result, "recent_orders", "table")
}

func TestPostgresCompleteSelectAliasesByClause(t *testing.T) {
	driver := &postgresDriver{}
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "group by", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY ", want: true},
		{name: "order by", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY id ORDER BY ", want: true},
		{name: "having", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY id HAVING ", want: false},
		{name: "where", sql: "SELECT id, SUM(id) AS total_amount FROM users WHERE ", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := driver.Complete(context.Background(), completer.Request{
				SQL: test.sql, CursorOffset: len(test.sql), TriggerKind: completer.TriggerInvoked,
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.want {
				requireCompletion(t, result, "total_amount", "column")
			} else {
				requireNoCompletion(t, result, "total_amount", "column")
			}
		})
	}
}

func TestPostgresCompletionContextMatrix(t *testing.T) {
	driver := &postgresDriver{}
	directory := completionTestCatalog("postgres", "public")
	metadata := &metadata.MetadataSet{Directory: directory, Objects: completionTestObjects("public")}
	tests := []struct {
		name           string
		sql            string
		requireLabel   string
		requireKind    string
		firstKind      string
		excludeRoutine bool
	}{
		{name: "select list", sql: "SELECT | FROM users", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "select relation", sql: "SELECT * FROM |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "join relation", sql: "SELECT * FROM users u JOIN |", requireLabel: "Order Items", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "where expression", sql: "SELECT * FROM users WHERE |", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "order expression", sql: "SELECT * FROM users ORDER BY |", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "insert relation", sql: "INSERT INTO |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "insert columns", sql: "INSERT INTO users (|)", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "update relation", sql: "UPDATE | SET id = 1", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "update columns", sql: "UPDATE users SET |", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "update predicate", sql: "UPDATE users SET id = 1 WHERE |", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "delete relation", sql: "DELETE FROM |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "delete predicate", sql: "DELETE FROM users WHERE |", requireLabel: "id", requireKind: "column", firstKind: "column"},
		{name: "alter relation", sql: "ALTER TABLE |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "alter action", sql: "ALTER TABLE users |", requireLabel: "ADD", requireKind: "keyword", excludeRoutine: true},
		{name: "alter add", sql: "ALTER TABLE users ADD |", requireLabel: "COLUMN", requireKind: "keyword", excludeRoutine: true},
		{name: "alter column name", sql: "ALTER TABLE users ADD COLUMN |", excludeRoutine: true},
		{name: "create table name", sql: "CREATE TABLE |", excludeRoutine: true},
		{name: "create index relation", sql: "CREATE INDEX users_idx ON |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "drop relation", sql: "DROP TABLE |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "truncate relation", sql: "TRUNCATE TABLE |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
		{name: "comment relation", sql: "COMMENT ON TABLE |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cursor := strings.IndexByte(test.sql, '|')
			sql := strings.Replace(test.sql, "|", "", 1)
			result, err := driver.Complete(context.Background(), completer.Request{
				SQL: sql, CursorOffset: cursor, Schema: metadata,
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.requireLabel != "" {
				requireCompletion(t, result, test.requireLabel, test.requireKind)
			}
			if test.firstKind != "" && (len(result.Suggestions) == 0 || result.Suggestions[0].Kind != test.firstKind) {
				t.Fatalf("first completion kind = %q, want %q: %+v", firstSuggestionKind(result), test.firstKind, result.Suggestions)
			}
			if test.excludeRoutine {
				requireNoSuggestionKinds(t, result, "function", "procedure")
			}
		})
	}
}

func completionTestCatalog(dialect, namespace string) *metadata.Directory {
	scope := completionTestScope(namespace)
	return &metadata.Directory{
		Engine: dialect, DefaultScope: scope,
		Roots: []metadata.ScopeNode{{
			Path: scope,
			Groups: []metadata.ObjectGroup{{
				Kind: "table",
				Objects: []metadata.ObjectRef{
					{Scope: scope, Kind: "table", Name: "users"},
					{Scope: scope, Kind: "table", Name: "Order Items"},
				},
			}},
		}},
	}
}

func completionTestObjects(namespace string) []metadata.Object {
	scope := completionTestScope(namespace)
	return []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "users"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "bigint"},
				{Name: "display name", DataType: "unsupported;type"},
			}},
		},
		{
			Ref:        metadata.ObjectRef{Scope: scope, Kind: "table", Name: "Order Items"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "bigint"}}},
		},
	}
}

func completionTestScope(namespace string) metadata.ScopePath {
	return metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: "app"},
		metadata.ScopeSegment{Kind: "schema", Name: namespace},
	)
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

func firstSuggestionKind(result completer.Result) string {
	if len(result.Suggestions) == 0 {
		return ""
	}
	return result.Suggestions[0].Kind
}

func requireNoSuggestionKinds(t *testing.T, result completer.Result, kinds ...string) {
	t.Helper()
	excluded := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		excluded[kind] = true
	}
	for _, suggestion := range result.Suggestions {
		if excluded[suggestion.Kind] {
			t.Fatalf("unexpected %s completion %q in %+v", suggestion.Kind, suggestion.Label, result.Suggestions)
		}
	}
}
