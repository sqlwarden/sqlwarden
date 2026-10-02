package sqlserver

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/schema"
)

type completionTestLoader struct {
	nav     *schema.Navigator
	conn    schema.Connection
	tree    metadata.Tree
	live    metadata.SchemaInspector
	ensured *int
}

func (l completionTestLoader) View(ctx context.Context) (*metadata.CompletionView, error) {
	return l.nav.CompletionView(ctx, l.conn, l.tree, l.live)
}

func (l completionTestLoader) Live() bool { return l.live != nil }

func (l completionTestLoader) Ensure(ctx context.Context, demands []metadata.Demand) completion.LoadReport {
	*l.ensured++
	return completion.LoadReport(l.nav.EnsureForCompletion(ctx, l.conn, l.tree, l.live, demands))
}

func TestCompletionResolvesObjectsAgainstLiveConnection(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	var database string
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	dropAll := func() {
		for _, stmt := range []string{
			`DROP PROCEDURE IF EXISTS cmp_sales.cmp_place`,
			`DROP FUNCTION IF EXISTS cmp_sales.cmp_open`,
			`DROP FUNCTION IF EXISTS dbo.cmp_tax`,
			`DROP SYNONYM IF EXISTS cmp_sales.cmp_syn`,
			`DROP VIEW IF EXISTS cmp_sales.cmp_order_view`,
			`DROP TABLE IF EXISTS dbo.cmp_customers`,
			`DROP TABLE IF EXISTS dbo.cmp_orders`,
			`DROP TABLE IF EXISTS cmp_sales.cmp_orders`,
			`DROP SCHEMA IF EXISTS cmp_sales`,
		} {
			_, _ = d.Execute(context.Background(), stmt)
		}
	}
	dropAll()
	t.Cleanup(dropAll)
	for _, stmt := range []string{
		`CREATE SCHEMA cmp_sales`,
		`CREATE TABLE cmp_sales.cmp_orders (id INT, total DECIMAL(10,2))`,
		`CREATE TABLE dbo.cmp_orders (legacy_id INT)`,
		`CREATE TABLE dbo.cmp_customers (id INT, name NVARCHAR(50))`,
		`CREATE VIEW cmp_sales.cmp_order_view AS SELECT id FROM cmp_sales.cmp_orders`,
		`CREATE SYNONYM cmp_sales.cmp_syn FOR dbo.cmp_customers`,
		`CREATE FUNCTION dbo.cmp_tax(@v DECIMAL(10,2)) RETURNS DECIMAL(10,2) AS BEGIN RETURN @v END`,
		`CREATE FUNCTION cmp_sales.cmp_open() RETURNS TABLE AS RETURN SELECT id FROM cmp_sales.cmp_orders`,
		`CREATE PROCEDURE cmp_sales.cmp_place AS SELECT 1`,
	} {
		mustExec(t, d, stmt)
	}

	defaultScope := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: database},
		metadata.ScopeSegment{Kind: "schema", Name: "cmp_sales"},
	)
	nav := schema.NewNavigator(nil, nil)
	ensured := 0
	complete := func(t *testing.T, sql string) (completer.Result, completion.Outcome) {
		t.Helper()
		ensured = 0
		loader := completionTestLoader{
			nav:     nav,
			conn:    schema.Connection{ID: 1, DefaultScope: defaultScope},
			tree:    d.Tree(),
			live:    d,
			ensured: &ensured,
		}
		result, outcome, err := completion.NewService().CompleteWithMetadata(ctx, "sqlserver", completer.Request{SQL: sql, CursorOffset: len(sql)}, loader)
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		return result, outcome
	}
	find := func(result completer.Result, kind, label string) (completer.Suggestion, bool) {
		for _, suggestion := range result.Suggestions {
			if suggestion.Kind == kind && suggestion.Label == label {
				return suggestion, true
			}
		}
		return completer.Suggestion{}, false
	}

	t.Run("unqualified relations shadow dbo", func(t *testing.T) {
		result, outcome := complete(t, "SELECT * FROM cmp_")
		if outcome.Status != completion.MetadataReady {
			t.Fatalf("outcome = %+v", outcome)
		}
		orders, ok := find(result, "table", "cmp_orders")
		if !ok || orders.Detail != "cmp_sales · table" {
			t.Fatalf("cmp_orders = %+v ok %v in %+v", orders, ok, result.Suggestions)
		}
		for _, want := range [][2]string{{"table", "cmp_customers"}, {"view", "cmp_order_view"}, {"synonym", "cmp_syn"}, {"function", "cmp_open"}} {
			if _, ok := find(result, want[0], want[1]); !ok {
				t.Fatalf("missing %s %s in %+v", want[0], want[1], result.Suggestions)
			}
		}
		if _, ok := find(result, "function", "cmp_tax"); ok {
			t.Fatal("scalar function offered as a row source")
		}
	})

	t.Run("columns and qualified scalar functions", func(t *testing.T) {
		result, outcome := complete(t, "SELECT * FROM cmp_orders o WHERE ")
		if outcome.Status != completion.MetadataReady {
			t.Fatalf("outcome = %+v", outcome)
		}
		for _, column := range []string{"id", "total"} {
			if _, ok := find(result, "column", column); !ok {
				t.Fatalf("missing column %s in %+v", column, result.Suggestions)
			}
		}
		if _, ok := find(result, "column", "legacy_id"); ok {
			t.Fatal("dbo.cmp_orders columns leaked through shadowing")
		}
		if tax, ok := find(result, "function", "cmp_tax"); !ok || tax.InsertText != "dbo.cmp_tax" {
			t.Fatalf("cmp_tax = %+v ok %v", tax, ok)
		}
	})

	t.Run("schema qualified", func(t *testing.T) {
		result, _ := complete(t, "SELECT * FROM cmp_sales.")
		if _, ok := find(result, "table", "cmp_orders"); !ok {
			t.Fatalf("missing cmp_orders in %+v", result.Suggestions)
		}
		if _, ok := find(result, "table", "cmp_customers"); ok {
			t.Fatal("dbo table under cmp_sales qualifier")
		}
	})

	t.Run("procedures after EXEC", func(t *testing.T) {
		result, _ := complete(t, "EXEC cmp_")
		if _, ok := find(result, "procedure", "cmp_place"); !ok {
			t.Fatalf("missing cmp_place in %+v", result.Suggestions)
		}
	})

	t.Run("hidden database yields nothing", func(t *testing.T) {
		result, _ := complete(t, "SELECT * FROM tempdb.dbo.")
		for _, suggestion := range result.Suggestions {
			if suggestion.Kind == "table" {
				t.Fatalf("table from hidden database: %+v", suggestion)
			}
		}
		if ensured != 0 {
			t.Fatalf("hidden database triggered %d metadata load attempts", ensured)
		}
	})
}

func TestCurrentScopeReportsSessionDatabaseAndSchema(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	var database string
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	got, err := d.CurrentScope(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database}, metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	if got != want {
		t.Fatalf("current scope = %q, want %q", got, want)
	}
}
