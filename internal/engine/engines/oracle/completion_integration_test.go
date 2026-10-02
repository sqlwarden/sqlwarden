//go:build integration

package oracle

import (
	"context"
	"database/sql"
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
	dropQuietly(d, `DROP TABLE COMPLETION_ORDERS`)
	t.Cleanup(func() { dropQuietly(d, `DROP TABLE COMPLETION_ORDERS`) })
	mustExec(t, d, `CREATE TABLE COMPLETION_ORDERS (ID NUMBER PRIMARY KEY, TOTAL NUMBER)`)
	nav := schema.NewNavigator(nil, nil)
	loader := completionTestLoader{
		nav:  nav,
		conn: schema.Connection{ID: 1, DefaultScope: oracleSchemaScope(oracleITSchema)},
		tree: d.Tree(),
		live: d,
	}
	svc := completion.NewService()

	for _, tc := range []struct{ sql, want string }{
		{"SELECT * FROM WARDEN.", "COMPLETION_ORDERS"},
		{"SELECT o. FROM WARDEN.COMPLETION_ORDERS o", "TOTAL"},
	} {
		cursor := strings.Index(tc.sql, ". ") + 1
		if cursor == 0 {
			cursor = len(tc.sql)
		}
		result, outcome, err := svc.CompleteWithMetadata(ctx, "oracle", completer.Request{SQL: tc.sql, CursorOffset: cursor}, loader)
		if err != nil {
			t.Fatal(err)
		}
		if !outcome.Loaded || !hasCompletionLabel(result, tc.want) {
			t.Fatalf("%q: missing %q (outcome %+v, suggestions %+v)", tc.sql, tc.want, outcome, result.Suggestions)
		}
	}
}

func TestCompletionStaysInCurrentSchemaUnlessAllSchemasShown(t *testing.T) {
	ctx := context.Background()
	sys, err := sql.Open("oracle", testSysDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sys.Close() })
	sysExec := func(stmt string) {
		t.Helper()
		if _, err := sys.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	_, _ = sys.ExecContext(ctx, `DROP USER COMPLETION_OTHER CASCADE`)
	t.Cleanup(func() { _, _ = sys.ExecContext(ctx, `DROP USER COMPLETION_OTHER CASCADE`) })
	sysExec(`CREATE USER COMPLETION_OTHER IDENTIFIED BY other_pass QUOTA UNLIMITED ON USERS`)
	sysExec(`CREATE TABLE COMPLETION_OTHER.COMPLETION_FOREIGN_ITEMS (ID NUMBER PRIMARY KEY)`)
	sysExec(`GRANT SELECT ON COMPLETION_OTHER.COMPLETION_FOREIGN_ITEMS TO WARDEN`)

	d := newConnectedDriver(t)
	dropQuietly(d, `DROP TABLE COMPLETION_MINE`)
	t.Cleanup(func() { dropQuietly(d, `DROP TABLE COMPLETION_MINE`) })
	mustExec(t, d, `CREATE TABLE COMPLETION_MINE (ID NUMBER PRIMARY KEY)`)

	complete := func(t *testing.T, loader completionTestLoader, sql string) (completer.Result, completion.Outcome) {
		t.Helper()
		result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "oracle", completer.Request{SQL: sql, CursorOffset: len(sql)}, loader)
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		return result, outcome
	}
	loaderFor := func(scope metadata.ScopePath, showAll bool) completionTestLoader {
		return completionTestLoader{
			nav:  schema.NewNavigator(nil, nil),
			conn: schema.Connection{ID: 1, DefaultScope: scope, ShowAllDatabases: showAll},
			tree: d.Tree(),
			live: d,
		}
	}

	t.Run("current schema only", func(t *testing.T) {
		loader := loaderFor(oracleSchemaScope(oracleITSchema), false)
		result, outcome := complete(t, loader, "SELECT * FROM ")
		if !hasCompletionLabel(result, "COMPLETION_MINE") {
			t.Fatalf("missing COMPLETION_MINE (outcome %+v, suggestions %+v)", outcome, result.Suggestions)
		}
		if hasCompletionLabel(result, "COMPLETION_FOREIGN_ITEMS") {
			t.Fatalf("other schema's table offered unqualified (outcome %+v)", outcome)
		}

		result, outcome = complete(t, loader, "SELECT * FROM COMPLETION_OTHER.")
		if hasCompletionLabel(result, "COMPLETION_FOREIGN_ITEMS") {
			t.Fatalf("other schema's table offered with show-all off (outcome %+v)", outcome)
		}
	})

	for _, scope := range []struct {
		name  string
		scope metadata.ScopePath
	}{{"configured default scope", oracleSchemaScope(oracleITSchema)}, {"no default scope", ""}} {
		t.Run(scope.name+"/all schemas shown", func(t *testing.T) {
			loader := loaderFor(scope.scope, true)
			result, outcome := complete(t, loader, "SELECT * FROM ")
			if !hasCompletionLabel(result, "COMPLETION_MINE") {
				t.Fatalf("missing COMPLETION_MINE (outcome %+v, suggestions %+v)", outcome, result.Suggestions)
			}
			if hasCompletionLabel(result, "COMPLETION_FOREIGN_ITEMS") {
				t.Fatalf("other schema's table offered unqualified (outcome %+v)", outcome)
			}

			result, outcome = complete(t, loader, "SELECT * FROM COMPLETION_OTHER.")
			if !hasCompletionLabel(result, "COMPLETION_FOREIGN_ITEMS") {
				t.Fatalf("missing COMPLETION_FOREIGN_ITEMS (outcome %+v, suggestions %+v)", outcome, result.Suggestions)
			}
		})
	}
}

func TestCurrentScopeReportsSessionSchema(t *testing.T) {
	d := newConnectedDriver(t)
	got, err := d.CurrentScope(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := oracleSchemaScope(oracleITSchema); got != want {
		t.Fatalf("current scope = %q, want %q", got, want)
	}
}
