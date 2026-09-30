package mariadb

import (
	"context"
	"strings"
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

func navNames(children []metadata.Child) map[string]metadata.Child {
	out := map[string]metadata.Child{}
	for _, c := range children {
		out[c.Kind+":"+c.Name] = c
	}
	return out
}

func navExec(t *testing.T, d *driver, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := d.DB().ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

var navDB = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})

func TestNavigatorListsSequencesApart(t *testing.T) {
	d := connect(t)
	navExec(t, d,
		`CREATE SEQUENCE nav_seq`,
		`CREATE TABLE nav_versioned (id INT PRIMARY KEY) WITH SYSTEM VERSIONING`,
	)
	t.Cleanup(func() { navExec(t, d, `DROP SEQUENCE IF EXISTS nav_seq`, `DROP TABLE IF EXISTS nav_versioned`) })
	sequences := navNames(navList(t, d, "database", "sequences", navDB))
	if _, ok := sequences["sequence:nav_seq"]; !ok {
		t.Fatalf("sequences = %v, want nav_seq", sequences)
	}
	tables := navNames(navList(t, d, "database", "tables", navDB))
	if _, ok := tables["table:nav_seq"]; ok {
		t.Error("tables must not list sequences")
	}
	if _, ok := tables["table:nav_versioned"]; !ok {
		t.Errorf("tables = %v, want system-versioned nav_versioned", tables)
	}
}

func TestNavigatorReportsJSONColumnType(t *testing.T) {
	d := connect(t)
	navExec(t, d, `CREATE TABLE nav_docs (id INT PRIMARY KEY, payload JSON, note LONGTEXT)`)
	t.Cleanup(func() { navExec(t, d, `DROP TABLE IF EXISTS nav_docs`) })
	table := navDB.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_docs"})
	columns := navNames(navList(t, d, "table", "columns", table))
	if got := columns["column:payload"].Attributes["data_type"]; got != "json" {
		t.Errorf("payload data_type = %v, want json", got)
	}
	if got := columns["column:note"].Attributes["data_type"]; got != "longtext" {
		t.Errorf("note data_type = %v, want longtext", got)
	}
}

func TestMariaDBNavigatorContract(t *testing.T) {
	d := connect(t)
	navExec(t, d,
		`CREATE TABLE nav_parent (id INT PRIMARY KEY, doc JSON)`,
		`CREATE TABLE nav_child (id INT PRIMARY KEY, parent_id INT, CONSTRAINT nav_child_parent FOREIGN KEY (parent_id) REFERENCES nav_parent (id))`,
	)
	t.Cleanup(func() { navExec(t, d, `DROP TABLE IF EXISTS nav_child`, `DROP TABLE IF EXISTS nav_parent`) })
	tables := []metadata.ScopePath{
		navDB.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_parent"}),
		navDB.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_child"}),
	}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "database", Folder: "sequences", Parents: []metadata.ScopePath{navDB}},
	}
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "references", "indexes"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "testdb", cases)
}

func TestConstraintDefinitionScopesCheckClauseToTable(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	navExec(t, d,
		`DROP TABLE IF EXISTS nav_chk_a`,
		`DROP TABLE IF EXISTS nav_chk_b`,
		`CREATE TABLE nav_chk_a (qty INT, CHECK (qty > 0))`,
		`CREATE TABLE nav_chk_b (qty INT, CHECK (qty < 100))`,
	)
	t.Cleanup(func() { navExec(t, d, `DROP TABLE IF EXISTS nav_chk_a`, `DROP TABLE IF EXISTS nav_chk_b`) })

	var name string
	if err := d.DB().QueryRowContext(ctx, `SELECT constraint_name FROM information_schema.table_constraints
WHERE table_schema = 'testdb' AND table_name = 'nav_chk_a' AND constraint_type = 'CHECK'`).Scan(&name); err != nil {
		t.Fatal(err)
	}

	tableRef := metadata.ObjectRef{Kind: "constraint", Name: name, Scope: navDB.Child(metadata.ScopeSegment{Kind: "table", Name: "nav_chk_a"})}
	desc, err := d.InspectDefinition(ctx, tableRef)
	if err != nil {
		t.Fatal(err)
	}
	got := descriptorText(t, desc)
	for _, want := range []string{"nav_chk_a", "> 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("table-scoped DDL = %q, want %q", got, want)
		}
	}
	for _, bad := range []string{"nav_chk_b", "< 100"} {
		if strings.Contains(got, bad) {
			t.Errorf("table-scoped DDL = %q, must not contain %q", got, bad)
		}
	}

	dbRef := metadata.ObjectRef{Kind: "constraint", Name: name, Scope: navDB}
	desc, err = d.InspectDefinition(ctx, dbRef)
	if err != nil {
		t.Fatal(err)
	}
	stmts := strings.Split(descriptorText(t, desc), "\n\n")
	if len(stmts) != 2 {
		t.Fatalf("database-scoped DDL has %d statements, want 2: %v", len(stmts), stmts)
	}
	for _, stmt := range stmts {
		switch {
		case strings.Contains(stmt, "nav_chk_a"):
			if !strings.Contains(stmt, "> 0") || strings.Contains(stmt, "< 100") {
				t.Errorf("nav_chk_a statement has wrong clause: %q", stmt)
			}
		case strings.Contains(stmt, "nav_chk_b"):
			if !strings.Contains(stmt, "< 100") || strings.Contains(stmt, "> 0") {
				t.Errorf("nav_chk_b statement has wrong clause: %q", stmt)
			}
		default:
			t.Errorf("unexpected statement: %q", stmt)
		}
	}
}

func descriptorText(t *testing.T, desc *metadata.Descriptor) string {
	t.Helper()
	if desc == nil || desc.Source == nil {
		t.Fatalf("descriptor = %+v, want source", desc)
	}
	return desc.Source.Body
}
