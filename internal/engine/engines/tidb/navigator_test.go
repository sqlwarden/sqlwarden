package tidb

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func navList(t *testing.T, d *driver, nodeKind, folderKind string, parent metadata.ScopePath) []metadata.Child {
	t.Helper()
	folder, ok := d.Tree().Folder(nodeKind, folderKind)
	if !ok {
		t.Fatalf("folder %q under %q not declared", folderKind, nodeKind)
	}
	q, err := d.Querier(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(context.Background(), q, []metadata.ScopePath{parent})
	if err != nil {
		t.Fatalf("%s/%s: %v", nodeKind, folderKind, err)
	}
	return out[parent]
}

func navExec(t *testing.T, d *driver, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := d.DB().ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestNavigatorTreeOmitsUnsupportedFolders(t *testing.T) {
	tree := (&driver{}).Tree()
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, f := range [][2]string{{"database", "procedures"}, {"database", "triggers"}, {"database", "events"}, {"table", "triggers"}} {
		if _, ok := tree.Folder(f[0], f[1]); ok {
			t.Errorf("%s/%s must not be declared", f[0], f[1])
		}
	}
	if _, ok := tree.Folder("database", "sequences"); !ok {
		t.Error("database/sequences must be declared")
	}
}

func TestNavigatorMarksTiDBSystemDatabases(t *testing.T) {
	d := connect(t)
	system := map[string]bool{}
	for _, c := range navList(t, d, "", "databases", "") {
		system[c.Name] = c.System
	}
	for _, name := range []string{"INFORMATION_SCHEMA", "METRICS_SCHEMA", "PERFORMANCE_SCHEMA", "mysql"} {
		if on, listed := system[name]; listed && !on {
			t.Errorf("%s must be system", name)
		}
	}
	if !system["METRICS_SCHEMA"] {
		t.Errorf("METRICS_SCHEMA missing or not system: %v", system)
	}
	if system["testdb"] {
		t.Error("testdb must not be system")
	}
}

func TestNavigatorListsSequences(t *testing.T) {
	d := connect(t)
	navExec(t, d, `CREATE SEQUENCE testdb.nav_seq`)
	t.Cleanup(func() { navExec(t, d, `DROP SEQUENCE IF EXISTS testdb.nav_seq`) })
	db := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	found := false
	for _, c := range navList(t, d, "database", "sequences", db) {
		found = found || (c.Kind == "sequence" && c.Name == "nav_seq")
	}
	if !found {
		t.Fatal("sequences missing nav_seq")
	}
	for _, c := range navList(t, d, "database", "tables", db) {
		if c.Name == "nav_seq" {
			t.Fatal("tables must not list sequences")
		}
	}
}

func TestTiDBNavigatorContract(t *testing.T) {
	d := connect(t)
	navExec(t, d,
		`CREATE TABLE testdb.nav_parent (id INT PRIMARY KEY, name VARCHAR(10) UNIQUE)`,
		`CREATE TABLE testdb.nav_child (id INT PRIMARY KEY, parent_id INT, CONSTRAINT nav_child_parent FOREIGN KEY (parent_id) REFERENCES testdb.nav_parent (id))`,
	)
	t.Cleanup(func() {
		navExec(t, d, `DROP TABLE IF EXISTS testdb.nav_child`, `DROP TABLE IF EXISTS testdb.nav_parent`)
	})
	db := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	tables := []metadata.ScopePath{
		db.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_parent"}),
		db.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_child"}),
	}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "database", Folder: "tables", Parents: []metadata.ScopePath{db}},
		{NodeKind: "database", Folder: "sequences", Parents: []metadata.ScopePath{db}},
	}
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "references", "indexes", "partitions"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "testdb", cases)
}
