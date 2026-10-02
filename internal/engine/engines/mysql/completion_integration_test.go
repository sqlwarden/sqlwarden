package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	mysqlconfig "github.com/go-sql-driver/mysql"
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
	d := connectAs(t, func(c *mysqlconfig.Config) { c.User = "root" })
	mustExecMySQL(t, d, `CREATE DATABASE shop`)
	t.Cleanup(func() { mustExecMySQL(t, d, `DROP DATABASE IF EXISTS shop`) })
	mustExecMySQL(t, d, `CREATE TABLE shop.orders (id int primary key, total decimal(10,2))`)
	nav := schema.NewNavigator(nil, nil)
	loader := completionTestLoader{
		nav:  nav,
		conn: schema.Connection{ID: 1, DefaultScope: navDatabase("testdb"), ShowAllDatabases: true},
		tree: d.Tree(),
		live: d,
	}
	svc := completion.NewService()

	for _, tc := range []struct{ sql, want string }{
		{"SELECT * FROM shop.", "orders"},
		{"SELECT o. FROM shop.orders o", "total"},
	} {
		cursor := strings.Index(tc.sql, ". ") + 1
		if cursor == 0 {
			cursor = len(tc.sql)
		}
		result, outcome, err := svc.CompleteWithMetadata(ctx, "mysql", completer.Request{SQL: tc.sql, CursorOffset: cursor}, loader)
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
	mustExecMySQL(t, d, `CREATE TABLE orders (id int primary key, total decimal(10,2))`)
	t.Cleanup(func() { mustExecMySQL(t, d, `DROP TABLE IF EXISTS orders`) })

	for _, scope := range []struct {
		name  string
		scope metadata.ScopePath
	}{{"no default scope", ""}, {"database only", navDatabase("testdb")}} {
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
			result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "mysql", completer.Request{SQL: tc.sql, CursorOffset: len(tc.sql)}, loader)
			if err != nil {
				t.Fatal(err)
			}
			if !outcome.Loaded || !hasCompletionLabel(result, tc.want) {
				t.Fatalf("%s %q: missing %q (outcome %+v, suggestions %+v)", scope.name, tc.sql, tc.want, outcome, result.Suggestions)
			}
		}
	}
}

func TestCompletionHonorsGrantsOfNonRootUser(t *testing.T) {
	ctx := context.Background()
	root := connectAs(t, func(c *mysqlconfig.Config) { c.User = "root" })
	mustExecMySQL(t, root, `CREATE DATABASE shop2`)
	mustExecMySQL(t, root, `CREATE DATABASE hidden_db`)
	mustExecMySQL(t, root, `CREATE USER 'completion_reader'@'%' IDENTIFIED BY 'readerpass'`)
	t.Cleanup(func() {
		mustExecMySQL(t, root, `DROP USER IF EXISTS 'completion_reader'@'%'`)
		mustExecMySQL(t, root, `DROP DATABASE IF EXISTS shop2`)
		mustExecMySQL(t, root, `DROP DATABASE IF EXISTS hidden_db`)
	})
	mustExecMySQL(t, root, `CREATE TABLE shop2.products (id int primary key, sku varchar(32))`)
	mustExecMySQL(t, root, `CREATE TABLE hidden_db.secrets (id int primary key, token varchar(64))`)
	mustExecMySQL(t, root, `GRANT SELECT ON shop2.* TO 'completion_reader'@'%'`)

	reader := connectAs(t, func(c *mysqlconfig.Config) {
		c.User = "completion_reader"
		c.Passwd = "readerpass"
		c.DBName = ""
	})
	complete := func(t *testing.T, loader completionTestLoader, sql string) (completer.Result, completion.Outcome) {
		t.Helper()
		result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "mysql", completer.Request{SQL: sql, CursorOffset: len(sql)}, loader)
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		return result, outcome
	}

	t.Run("default scope shop2", func(t *testing.T) {
		loader := completionTestLoader{
			nav:  schema.NewNavigator(nil, nil),
			conn: schema.Connection{ID: 1, DefaultScope: navDatabase("shop2"), ShowAllDatabases: true},
			tree: reader.Tree(),
			live: reader,
		}
		result, outcome := complete(t, loader, "select * from ")
		if !outcome.Loaded || !hasCompletionLabel(result, "products") {
			t.Fatalf("missing products (outcome %+v, suggestions %+v)", outcome, result.Suggestions)
		}
		if hasCompletionLabel(result, "secrets") || hasCompletionLabel(result, "hidden_db") {
			t.Fatalf("ungranted objects offered: %+v", result.Suggestions)
		}

		result, outcome = complete(t, loader, "select * from hidden_db.")
		if hasCompletionLabel(result, "secrets") {
			t.Fatalf("hidden_db.secrets offered (outcome %+v)", outcome)
		}
		if outcome.Status == completion.MetadataDegraded {
			t.Fatalf("outcome = %+v", outcome)
		}

		listing, err := loader.nav.Children(ctx, loader.conn, loader.tree, reader, "", "databases")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := childNamed(listing.Items, "database", "hidden_db"); ok {
			t.Fatalf("root listing surfaced hidden_db: %+v", listing.Items)
		}
		if _, ok := childNamed(listing.Items, "database", "shop2"); !ok {
			t.Fatalf("root listing missing shop2: %+v", listing.Items)
		}
	})

	t.Run("no default scope and no session database", func(t *testing.T) {
		var current sql.NullString
		if err := reader.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current.Valid {
			t.Fatalf("DATABASE() = %q, want NULL", current.String)
		}
		for _, showAll := range []bool{false, true} {
			loader := completionTestLoader{
				nav:  schema.NewNavigator(nil, nil),
				conn: schema.Connection{ID: 1, ShowAllDatabases: showAll},
				tree: reader.Tree(),
				live: reader,
			}
			result, outcome := complete(t, loader, "select * from ")
			if outcome.Status == completion.MetadataDegraded {
				t.Fatalf("show all %v: outcome = %+v", showAll, outcome)
			}
			if hasCompletionLabel(result, "secrets") {
				t.Fatalf("show all %v: hidden_db.secrets offered", showAll)
			}

			result, outcome = complete(t, loader, "select * from shop2.")
			if !hasCompletionLabel(result, "products") {
				t.Fatalf("show all %v: shop2. missing products (outcome %+v, suggestions %+v)", showAll, outcome, result.Suggestions)
			}
		}
	})
}

func TestCurrentScopeReportsSessionDatabase(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	got, err := d.CurrentScope(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := navDatabase("testdb"); got != want {
		t.Fatalf("current scope = %q, want %q", got, want)
	}

	none := connectAs(t, func(c *mysqlconfig.Config) { c.DBName = "" })
	got, err = none.CurrentScope(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("current scope without a session database = %q, want empty", got)
	}
}
