package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
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
		SQL: sql, CursorOffset: len("SELECT "),
		Schema:       &metadata.MetadataSet{Directory: catalog, Objects: objects, Version: "snapshot-1"},
		ConnectionID: "8",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "display name", "column")

	fromSQL := "SELECT * FROM "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: fromSQL, CursorOffset: len(fromSQL),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
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
	if got := mysqlQuoteCompletionIdentifier("item"); got != "item" {
		t.Fatalf("safe identifier insertion = %q", got)
	}
}

func TestMySQLCompleteCuratesCompletedRelationContext(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := mysqlCompletionTestObjects()

	sql := "SELECT * FROM users "
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerInvoked,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"AS", "JOIN", "STRAIGHT_JOIN", "WHERE", "GROUP", "ORDER", "LIMIT"} {
		requireMySQLCompletion(t, result, label, "keyword")
	}
	for _, label := range []string{"ALTER", "CREATE", "DATABASE", "ON", "USING"} {
		requireNoMySQLCompletion(t, result, label, "keyword")
	}

	joinedSQL := "SELECT * FROM users u JOIN `Order Items` oi "
	result, err = driver.Complete(context.Background(), completer.Request{
		SQL: joinedSQL, CursorOffset: len(joinedSQL),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerInvoked,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "ON", "keyword")
	requireMySQLCompletion(t, result, "USING", "keyword")
	requireNoMySQLCompletion(t, result, "AS", "keyword")
	requireNoMySQLCompletion(t, result, "ALTER", "keyword")
}

func TestMySQLCompleteUsesStatementAtCursor(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := mysqlCompletionTestObjects()
	sql := `select s.first_name, s.last_name, a.address from staff s
join store st
on s.staff_id = st.manager_staff_id
join address a
on a.address_id = s.address_id;

select * from `
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerAutomatic,
		TriggerChar: " ",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "users", "table")
}

func TestMySQLCompleteRecoversNewSelectWithoutSemicolon(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := mysqlCompletionTestObjects()
	sql := `select s.first_name, s.last_name, a.address from staff s
join store st
on s.staff_id = st.manager_staff_id
join address a
on a.address_id = s.address_id;

select * from actor

select * from `
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerAutomatic,
		TriggerChar: " ",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "users", "table")
}

func TestMySQLCompleteRespectsQualifiedAliasesAndJoinConflicts(t *testing.T) {
	driver := &mysqlDriver{}
	catalog := mysqlCompletionTestCatalog()
	objects := []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: mysqlCompletionTestScope(), Kind: "table", Name: "inventory"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "BIGINT"}, {Name: "inventory_name", DataType: "TEXT"},
			}},
		},
		{
			Ref: metadata.ObjectRef{Scope: mysqlCompletionTestScope(), Kind: "table", Name: "store"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "BIGINT"}, {Name: "store_name", DataType: "TEXT"},
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
	requireMySQLCompletion(t, qualified, "store_name", "column")
	requireNoMySQLCompletion(t, qualified, "inventory_name", "column")

	unqualifiedSQL := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE "
	unqualified, err := driver.Complete(context.Background(), completer.Request{
		SQL: unqualifiedSQL, CursorOffset: len(unqualifiedSQL),
		Schema: &metadata.MetadataSet{Directory: catalog, Objects: objects},
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
	objects := []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: mysqlCompletionTestScope(), Kind: "table", Name: "film"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "film_id", DataType: "SMALLINT"},
				{Name: "description", DataType: "TEXT"},
			}},
		},
		{
			Ref: metadata.ObjectRef{Scope: mysqlCompletionTestScope(), Kind: "table", Name: "film_actor"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "actor_id", DataType: "SMALLINT"},
				{Name: "film_id", DataType: "SMALLINT"},
			}},
		},
	}
	catalog.Roots[0].Groups[0].Objects = []metadata.ObjectRef{objects[0].Ref, objects[1].Ref}

	sql := "select * from film f\njoin film_actor fa\nwhere f.`description` = fa."
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema:      &metadata.MetadataSet{Directory: catalog, Objects: objects},
		TriggerKind: completer.TriggerAutomatic,
		TriggerChar: ".",
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

func TestMySQLSuggestionRankingPrioritizesTypedPrefix(t *testing.T) {
	suggestions := []completer.Suggestion{
		{Label: "consumer", Score: 100},
		{Label: "summary", Score: 1},
		{Label: "gross_sum", Score: 100},
		{Label: "SUM", Score: 1},
	}
	mysqlSortSuggestions(suggestions, "sum")
	want := []string{"SUM", "summary", "gross_sum", "consumer"}
	for i, label := range want {
		if suggestions[i].Label != label {
			t.Fatalf("suggestion %d = %q, want %q: %+v", i, suggestions[i].Label, label, suggestions)
		}
	}
}

func TestMySQLCompleteRefreshesFunctionPrefix(t *testing.T) {
	for _, prefix := range []string{"su", "sum"} {
		t.Run(prefix, func(t *testing.T) {
			sql := "SELECT customer_id, SUM(amount) AS total_amount FROM payment GROUP BY customer_id HAVING " + prefix
			result, err := (&mysqlDriver{}).Complete(context.Background(), completer.Request{
				SQL: sql, CursorOffset: len(sql), TriggerKind: completer.TriggerAutomatic,
			})
			if err != nil {
				t.Fatal(err)
			}
			requireMySQLCompletion(t, result, "SUM", "function")
			for _, suggestion := range result.Suggestions {
				if !strings.HasPrefix(strings.ToLower(suggestion.Label), prefix) {
					t.Fatalf("non-prefix suggestion %q returned for %q", suggestion.Label, prefix)
				}
			}
		})
	}
}

func TestMySQLCompleteSelectAliasesByClause(t *testing.T) {
	driver := &mysqlDriver{}
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "group by", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY ", want: true},
		{name: "having", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY id HAVING ", want: true},
		{name: "order by", sql: "SELECT id, SUM(id) AS total_amount FROM users GROUP BY id ORDER BY ", want: true},
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
				requireMySQLCompletion(t, result, "total_amount", "column")
			} else {
				requireNoMySQLCompletion(t, result, "total_amount", "column")
			}
		})
	}
}

func TestMySQLCompleteInsideStandaloneCTE(t *testing.T) {
	driver := &mysqlDriver{}
	set := &metadata.MetadataSet{
		Directory: mysqlCompletionTestCatalog(), Objects: mysqlCompletionTestObjects(),
	}
	for _, trigger := range []completer.TriggerKind{completer.TriggerAutomatic, completer.TriggerInvoked} {
		for _, suffix := range []string{"", "\n-- block\n-- SELECT\n-- FROM users\n-- block"} {
			name := string(trigger)
			if suffix != "" {
				name += "/commented-outer"
			}
			t.Run(name, func(t *testing.T) {
				template := "WITH picked AS (\n  SELECT \n    |\n  FROM users\n)" + suffix
				cursor := strings.IndexByte(template, '|')
				sql := strings.Replace(template, "|", "", 1)
				result, err := driver.Complete(context.Background(), completer.Request{
					SQL: sql, CursorOffset: cursor, TriggerKind: trigger, Schema: set,
				})
				if err != nil {
					t.Fatal(err)
				}
				requireMySQLCompletion(t, result, "id", "column")
				requireMySQLCompletion(t, result, "display name", "column")
			})
		}
	}
}

func TestMySQLCompleteCTERelationsAndProjectedColumns(t *testing.T) {
	driver := &mysqlDriver{}
	set := &metadata.MetadataSet{
		Directory: mysqlCompletionTestCatalog(), Objects: mysqlCompletionTestObjects(),
	}
	for _, trigger := range []completer.TriggerKind{completer.TriggerAutomatic, completer.TriggerInvoked} {
		for _, prefix := range []string{"", "pic"} {
			t.Run(string(trigger)+"/relation/"+prefix, func(t *testing.T) {
				sql := "WITH picked AS (SELECT id, `display name` AS amt FROM users) SELECT * FROM " + prefix
				result, err := driver.Complete(context.Background(), completer.Request{
					SQL: sql, CursorOffset: len(sql), TriggerKind: trigger, Schema: set,
				})
				if err != nil {
					t.Fatal(err)
				}
				requireMySQLCompletion(t, result, "picked", "table")
			})
		}
	}

	template := "WITH picked AS (SELECT id, `display name` AS amt FROM users) SELECT | FROM picked"
	cursor := strings.IndexByte(template, '|')
	sql := strings.Replace(template, "|", "", 1)
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: cursor, TriggerKind: completer.TriggerInvoked, Schema: set,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireMySQLCompletion(t, result, "id", "column")
	requireMySQLCompletion(t, result, "amt", "column")
	requireNoMySQLCompletion(t, result, "display name", "column")
}

func TestMySQLCompleteDoesNotEchoUnknownRelationPrefix(t *testing.T) {
	directory := mysqlCompletionTestCatalog()
	objects := mysqlCompletionTestObjects()
	sql := "SELECT * FROM veraxasdwadqwd"
	result, err := (&mysqlDriver{}).Complete(context.Background(), completer.Request{
		SQL: sql, CursorOffset: len(sql),
		Schema: &metadata.MetadataSet{Directory: directory, Objects: objects},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoMySQLCompletion(t, result, "veraxasdwadqwd", "table")
}

func TestMySQLCompletionContextMatrix(t *testing.T) {
	driver := &mysqlDriver{}
	directory := mysqlCompletionTestCatalog()
	metadata := &metadata.MetadataSet{Directory: directory, Objects: mysqlCompletionTestObjects()}
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
		{name: "rename relation", sql: "RENAME TABLE |", requireLabel: "users", requireKind: "table", firstKind: "table", excludeRoutine: true},
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
				requireMySQLCompletion(t, result, test.requireLabel, test.requireKind)
			}
			if test.firstKind != "" && (len(result.Suggestions) == 0 || result.Suggestions[0].Kind != test.firstKind) {
				t.Fatalf("first completion kind = %q, want %q: %+v", firstMySQLSuggestionKind(result), test.firstKind, result.Suggestions)
			}
			if test.excludeRoutine {
				requireNoMySQLSuggestionKinds(t, result, "function", "procedure")
			}
		})
	}
}

func mysqlCompletionTestCatalog() *metadata.Directory {
	scope := mysqlCompletionTestScope()
	return &metadata.Directory{
		Engine: "mysql", DefaultScope: scope,
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

func mysqlCompletionTestObjects() []metadata.Object {
	scope := mysqlCompletionTestScope()
	return []metadata.Object{
		{
			Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "users"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
				{Name: "id", DataType: "BIGINT"},
				{Name: "display name", DataType: "unsupported;type"},
			}},
		},
		{
			Ref:        metadata.ObjectRef{Scope: scope, Kind: "table", Name: "Order Items"},
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "BIGINT"}}},
		},
	}
}

func mysqlCompletionTestScope() metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
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

func firstMySQLSuggestionKind(result completer.Result) string {
	if len(result.Suggestions) == 0 {
		return ""
	}
	return result.Suggestions[0].Kind
}

func requireNoMySQLSuggestionKinds(t *testing.T, result completer.Result, kinds ...string) {
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
