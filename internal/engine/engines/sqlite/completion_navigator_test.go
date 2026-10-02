package sqlite

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
	d := navDriver(t, `CREATE TABLE orders (id integer primary key, total real)`)

	nav := schema.NewNavigator(nil, nil)
	loader := completionTestLoader{
		nav:  nav,
		conn: schema.Connection{ID: 1, DefaultScope: navDatabase("main")},
		tree: d.Tree(),
		live: d,
	}
	svc := completion.NewService()

	for _, tc := range []struct{ sql, want string }{
		{"SELECT * FROM ", "orders"},
		{"SELECT o. FROM orders o", "total"},
	} {
		cursor := strings.Index(tc.sql, ". ") + 1
		if cursor == 0 {
			cursor = len(tc.sql)
		}
		result, outcome, err := svc.CompleteWithMetadata(ctx, "sqlite", completer.Request{SQL: tc.sql, CursorOffset: cursor}, loader)
		if err != nil {
			t.Fatal(err)
		}
		if !outcome.Loaded || !hasCompletionLabel(result, tc.want) {
			t.Fatalf("%q: missing %q (outcome %+v, suggestions %+v)", tc.sql, tc.want, outcome, result.Suggestions)
		}
	}
}

func TestCurrentScopeIsMainDatabase(t *testing.T) {
	d := navDriver(t)
	got, err := d.CurrentScope(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := navDatabase("main"); got != want {
		t.Fatalf("current scope = %q, want %q", got, want)
	}
}

func TestCompletionUsesMainWithoutDefaultScope(t *testing.T) {
	ctx := context.Background()
	d := navDriver(t, `CREATE TABLE orders (id integer primary key, total real)`)
	loader := completionTestLoader{
		nav:  schema.NewNavigator(nil, nil),
		conn: schema.Connection{ID: 1},
		tree: d.Tree(),
		live: d,
	}
	sql := "SELECT * FROM "
	result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "sqlite", completer.Request{SQL: sql, CursorOffset: len(sql)}, loader)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := completionNamed(result, "orders")
	if !ok || got.InsertText != "orders" {
		t.Fatalf("orders = %+v %v (outcome %+v, suggestions %+v)", got, ok, outcome, result.Suggestions)
	}
}

func completionNamed(result completer.Result, label string) (completer.Suggestion, bool) {
	for _, s := range result.Suggestions {
		if s.Label == label {
			return s, true
		}
	}
	return completer.Suggestion{}, false
}
