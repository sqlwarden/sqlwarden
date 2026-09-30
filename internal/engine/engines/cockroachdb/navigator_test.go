package cockroachdb

import (
	"context"
	"database/sql"
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func mustExec(t *testing.T, d *driver, stmt string) {
	t.Helper()
	if _, err := d.DB().ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}

func currentDatabase(t *testing.T, d *driver) string {
	t.Helper()
	var name string
	if err := d.DB().QueryRowContext(context.Background(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func navDatabase(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
}

func navSchema(database, schema string) metadata.ScopePath {
	return navDatabase(database).Child(metadata.ScopeSegment{Kind: "schema", Name: schema})
}

func listFolder(t *testing.T, d *driver, database, nodeKind, folderKind string, parent metadata.ScopePath) map[string]metadata.Child {
	t.Helper()
	folder, ok := d.Tree().Folder(nodeKind, folderKind)
	if !ok {
		t.Fatalf("folder %s/%s not declared", nodeKind, folderKind)
	}
	q, err := d.Querier(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(context.Background(), q, []metadata.ScopePath{parent})
	if err != nil {
		t.Fatalf("%s/%s: %v", nodeKind, folderKind, err)
	}
	byName := map[string]metadata.Child{}
	for _, c := range out[parent] {
		byName[c.Name] = c
	}
	return byName
}

func TestNavigatorTreeOmitsUnsupportedFolders(t *testing.T) {
	tree := (&driver{}).Tree()
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, f := range [][2]string{
		{"database", "event_triggers"}, {"database", "extensions"},
		{"schema", "foreign_tables"}, {"schema", "aggregate_functions"},
		{"table", "partitions"}, {"table", "rules"}, {"view", "rules"},
	} {
		if _, ok := tree.Folder(f[0], f[1]); ok {
			t.Errorf("folder %s/%s must not be declared", f[0], f[1])
		}
	}
	for _, f := range [][2]string{
		{"schema", "tables"}, {"schema", "materialized_views"}, {"schema", "functions"},
		{"table", "triggers"}, {"table", "policies"}, {"materialized_view", "indexes"},
	} {
		if _, ok := tree.Folder(f[0], f[1]); !ok {
			t.Errorf("folder %s/%s must be declared", f[0], f[1])
		}
	}
}

func TestNavigatorListsTables(t *testing.T) {
	d := connect(t)
	mustExec(t, d, `CREATE SCHEMA nav_tables`)
	mustExec(t, d, `CREATE TABLE nav_tables.users (id int PRIMARY KEY)`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS nav_tables CASCADE`) })
	db := currentDatabase(t, d)

	tables := listFolder(t, d, db, "schema", "tables", navSchema(db, "nav_tables"))
	if _, ok := tables["users"]; !ok {
		t.Fatalf("tables = %v, want users", tables)
	}
}

func TestNavigatorMarksSystemObjects(t *testing.T) {
	d := connect(t)
	db := currentDatabase(t, d)

	schemas := listFolder(t, d, db, "database", "schemas", navDatabase(db))
	for _, name := range []string{"crdb_internal", "pg_extension", "pg_catalog", "information_schema"} {
		if !schemas[name].System {
			t.Errorf("schema %q must be system", name)
		}
	}
	if schemas["public"].System {
		t.Error("public must not be system")
	}

	databases := listFolder(t, d, db, "", "databases", "")
	if !databases["system"].System {
		t.Error("database system must be system")
	}
	if databases[db].System {
		t.Errorf("database %q must not be system", db)
	}
}

func TestInspectObjectsRoutesByDatabase(t *testing.T) {
	d := connect(t)
	mustExec(t, d, `CREATE DATABASE nav_other`)
	t.Cleanup(func() { mustExec(t, d, `DROP DATABASE IF EXISTS nav_other CASCADE`) })
	mustExec(t, d, `CREATE TABLE nav_other.public.widgets (id int PRIMARY KEY)`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW nav_other.public.widget_ids AS SELECT id FROM nav_other.public.widgets`)
	other, err := d.Querier(context.Background(), "nav_other")
	if err != nil {
		t.Fatal(err)
	}
	execer, ok := other.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	})
	if !ok {
		t.Fatalf("querier %T cannot exec", other)
	}
	for _, stmt := range []string{
		`CREATE FUNCTION public.answer() RETURNS int LANGUAGE SQL AS 'SELECT 42'`,
		`CREATE PROCEDURE public.touch() LANGUAGE SQL AS 'SELECT 1'`,
	} {
		if _, err := execer.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	scope := navSchema("nav_other", "public")
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "table", Name: "widgets"},
		{Scope: scope, Kind: "materialized_view", Name: "widget_ids"},
		{Scope: scope, Kind: "function", Name: "answer"},
		{Scope: scope, Kind: "procedure", Name: "touch"},
	}
	objs, err := d.InspectObjects(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]metadata.ObjectRef{}
	for _, o := range objs {
		found[o.Ref.Kind+":"+o.Ref.Name] = o.Ref
	}
	for _, ref := range refs {
		got, ok := found[ref.Kind+":"+ref.Name]
		if !ok {
			t.Fatalf("missing %s %s in %v", ref.Kind, ref.Name, found)
		}
		if got.Scope.Name("database") != "nav_other" {
			t.Errorf("%s scope = %q, want database nav_other", ref.Name, got.Scope)
		}
	}

	for _, ref := range refs[2:] {
		desc, err := d.InspectDefinition(context.Background(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if desc == nil {
			t.Fatalf("%s definition missing", ref.Kind)
		}
	}
}

func TestCockroachDBNavigatorContract(t *testing.T) {
	d := connect(t)
	mustExec(t, d, `CREATE SCHEMA nav_a`)
	mustExec(t, d, `CREATE SCHEMA nav_b`)
	mustExec(t, d, `CREATE TABLE nav_a.users (id int PRIMARY KEY, email text UNIQUE)`)
	mustExec(t, d, `CREATE TABLE nav_b.orders (id int PRIMARY KEY, user_id int REFERENCES nav_a.users(id))`)
	mustExec(t, d, `CREATE VIEW nav_a.user_ids AS SELECT id FROM nav_a.users`)
	t.Cleanup(func() {
		mustExec(t, d, `DROP SCHEMA IF EXISTS nav_b CASCADE`)
		mustExec(t, d, `DROP SCHEMA IF EXISTS nav_a CASCADE`)
	})
	db := currentDatabase(t, d)
	schemas := []metadata.ScopePath{navSchema(db, "nav_a"), navSchema(db, "nav_b")}
	tables := []metadata.ScopePath{
		navSchema(db, "nav_a").Child(metadata.ScopeSegment{Kind: "table", Name: "users"}),
		navSchema(db, "nav_b").Child(metadata.ScopeSegment{Kind: "table", Name: "orders"}),
	}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "database", Folder: "schemas", Parents: []metadata.ScopePath{navDatabase(db)}},
	}
	for _, folder := range []string{"tables", "views", "materialized_views", "indexes", "functions", "sequences", "data_types"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "schema", Folder: folder, Parents: schemas})
	}
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "indexes", "dependencies", "references", "triggers", "policies"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "", cases)
}
