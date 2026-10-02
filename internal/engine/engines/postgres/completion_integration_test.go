package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/schema"
)

type completionTestLoader struct {
	nav  *schema.Navigator
	conn schema.Connection
	tree metadata.Tree
	live metadata.SchemaInspector
}

func (l completionTestLoader) View(ctx context.Context) (*metadata.CompletionView, error) {
	return l.nav.CompletionView(ctx, l.conn, l.tree, l.live)
}

func (l completionTestLoader) Live() bool { return l.live != nil }

func (l completionTestLoader) Ensure(ctx context.Context, demands []metadata.Demand) completion.LoadReport {
	return completion.LoadReport(l.nav.EnsureForCompletion(ctx, l.conn, l.tree, l.live, demands))
}

func hasCompletionLabel(result completer.Result, label string) bool {
	for _, s := range result.Suggestions {
		if strings.EqualFold(s.Label, label) {
			return true
		}
	}
	return false
}

func TestCompletionFetchesNeverExpandedObjects(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE SCHEMA sales`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS sales CASCADE`) })
	mustExec(t, d, `CREATE TABLE sales.orders (id int primary key, total numeric)`)
	nav := schema.NewNavigator(nil, nil)
	loader := completionTestLoader{
		nav:  nav,
		conn: schema.Connection{ID: 1, DefaultScope: schemaPath(currentDatabase(t, d), "public")},
		tree: d.Tree(),
		live: d,
	}
	svc := completion.NewService()

	for _, tc := range []struct{ sql, want string }{
		{"SELECT * FROM sales.", "orders"},
		{"SELECT o. FROM sales.orders o", "total"},
	} {
		cursor := strings.Index(tc.sql, ". ") + 1
		if cursor == 0 {
			cursor = len(tc.sql)
		}
		result, outcome, err := svc.CompleteWithMetadata(ctx, "postgres", completer.Request{SQL: tc.sql, CursorOffset: cursor}, loader)
		if err != nil {
			t.Fatal(err)
		}
		if !outcome.Loaded || !hasCompletionLabel(result, tc.want) {
			t.Fatalf("%q: missing %q (outcome %+v, suggestions %+v)", tc.sql, tc.want, outcome, result.Suggestions)
		}
	}
}

func TestCompletionDerivesMissingDefaultScopeFromSession(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE TABLE orders (id int primary key, total numeric)`)
	t.Cleanup(func() { mustExec(t, d, `DROP TABLE IF EXISTS orders`) })
	database := schemaPath(currentDatabase(t, d), "public").Parent()

	for _, scope := range []struct {
		name  string
		scope metadata.ScopePath
	}{{"no default scope", ""}, {"database only", database}} {
		for _, tc := range []struct{ sql, want string }{
			{"SELECT * FROM orders WHERE ", "total"},
			{"SELECT * FROM orders o WHERE o.", "total"},
			{"SELECT * FROM ", "orders"},
		} {
			loader := completionTestLoader{
				nav:  schema.NewNavigator(nil, nil),
				conn: schema.Connection{ID: 1, DefaultScope: scope.scope},
				tree: d.Tree(),
				live: d,
			}
			result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "postgres", completer.Request{SQL: tc.sql, CursorOffset: len(tc.sql)}, loader)
			if err != nil {
				t.Fatal(err)
			}
			if !outcome.Loaded || !hasCompletionLabel(result, tc.want) {
				t.Fatalf("%s %q: missing %q (outcome %+v, suggestions %+v)", scope.name, tc.sql, tc.want, outcome, result.Suggestions)
			}
		}
	}
}

func TestCurrentScopeReportsSessionDatabaseAndSchema(t *testing.T) {
	d := newConnectedDriver(t)
	got, err := d.CurrentScope(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := schemaPath(currentDatabase(t, d), "public"); got != want {
		t.Fatalf("current scope = %q, want %q", got, want)
	}
}
