package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

// navDriver connects to a file database on a single pooled connection so
// ATTACH and temp objects stay visible to every query.
func navDriver(t *testing.T, stmts ...string) *sqliteDriver {
	t.Helper()
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: filepath.Join(t.TempDir(), "nav.db"), Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	d.db.SetMaxOpenConns(1)
	for _, stmt := range stmts {
		if _, err := d.db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return d
}

func navDatabase(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
}

func navRelation(database, kind, name string) metadata.ScopePath {
	return navDatabase(database).Child(metadata.ScopeSegment{Kind: kind, Name: name})
}

func navList(t *testing.T, d *sqliteDriver, nodeKind, folderKind string, parents ...metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	t.Helper()
	folder, ok := d.Tree().Folder(nodeKind, folderKind)
	if !ok {
		t.Fatalf("folder %q under %q not declared", folderKind, nodeKind)
	}
	q, err := d.Querier(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(context.Background(), q, parents)
	if err != nil {
		t.Fatalf("%s/%s: %v", nodeKind, folderKind, err)
	}
	return out
}

func childNamed(t *testing.T, children []metadata.Child, kind, name string) metadata.Child {
	t.Helper()
	for _, c := range children {
		if c.Kind == kind && c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s %q in %+v", kind, name, children)
	return metadata.Child{}
}

func childNames(children []metadata.Child) []string {
	names := make([]string, 0, len(children))
	for _, c := range children {
		names = append(names, c.Name)
	}
	return names
}

const navSchema = `
CREATE TABLE parent (id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT UNIQUE, label TEXT, UNIQUE (label, code));
CREATE TABLE child (
  id INT,
  parent_id INT REFERENCES Parent(id) ON DELETE CASCADE,
  doubled INT GENERATED ALWAYS AS (id * 2) VIRTUAL
);
CREATE TABLE pair (a INT, b INT, PRIMARY KEY (b, a), FOREIGN KEY (a, b) REFERENCES pair_target(x, y)) STRICT;
CREATE TABLE pair_target (x INT, y INT, UNIQUE (x, y));
CREATE INDEX child_parent_idx ON child (parent_id);
CREATE VIEW parent_view AS SELECT id, code FROM parent;
CREATE TRIGGER child_audit AFTER INSERT ON child BEGIN SELECT 1; END;
CREATE VIRTUAL TABLE notes USING fts5(body);
INSERT INTO parent (code, label) VALUES ('a', 'A'), ('b', 'B');`

func TestNavigatorTreeValidates(t *testing.T) {
	tree := (&sqliteDriver{}).Tree()
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	if tree.DatabaseKind() != "database" {
		t.Fatalf("DatabaseKind = %q, want database", tree.DatabaseKind())
	}
	table, _ := tree.Node("table")
	var folders []string
	for _, f := range table.Folders {
		folders = append(folders, f.Kind)
	}
	want := []string{"columns", "keys", "foreign_keys", "indexes", "references", "triggers"}
	if !reflect.DeepEqual(folders, want) {
		t.Fatalf("table folders = %v, want %v", folders, want)
	}
}

func TestQuerierRequiresConnection(t *testing.T) {
	if _, err := (&sqliteDriver{}).Querier(context.Background(), ""); err == nil {
		t.Fatal("Querier on an unconnected driver must fail")
	}
}

func TestListDatabasesFlagsTempAndMarksMainCurrent(t *testing.T) {
	d := navDriver(t, `ATTACH ':memory:' AS aux`, `CREATE TEMP TABLE scratch (id INT)`)
	dbs := navList(t, d, "", "databases", "")[""]
	if got := childNames(dbs); !reflect.DeepEqual(got, []string{"main", "temp", "aux"}) {
		t.Fatalf("databases = %v", got)
	}
	if main := childNamed(t, dbs, "database", "main"); !main.Current || main.System {
		t.Fatalf("main = %+v, want current and not system", main)
	}
	if temp := childNamed(t, dbs, "database", "temp"); !temp.System || temp.Current {
		t.Fatalf("temp = %+v, want system and not current", temp)
	}
	if aux := childNamed(t, dbs, "database", "aux"); aux.System || aux.Current {
		t.Fatalf("aux = %+v, want neither system nor current", aux)
	}
}

func TestListTablesFlagsInternalAndShadowTables(t *testing.T) {
	d := navDriver(t, navSchema)
	tables := navList(t, d, "database", "tables", navDatabase("main"))[navDatabase("main")]
	if parent := childNamed(t, tables, "table", "parent"); parent.System || parent.Attributes["strict"] != false {
		t.Fatalf("parent = %+v", parent)
	}
	if pair := childNamed(t, tables, "table", "pair"); pair.Attributes["strict"] != true {
		t.Fatalf("pair = %+v, want strict", pair)
	}
	if notes := childNamed(t, tables, "table", "notes"); notes.System || notes.Attributes["virtual"] != true {
		t.Fatalf("notes = %+v, want virtual and not system", notes)
	}
	for _, name := range []string{"sqlite_sequence", "notes_data", "notes_config"} {
		if c := childNamed(t, tables, "table", name); !c.System {
			t.Fatalf("%s = %+v, want system", name, c)
		}
	}
	for _, c := range tables {
		if c.Name == "parent_view" {
			t.Fatal("views must not be listed as tables")
		}
	}
}

func TestListDatabaseFoldersScopeToTheirDatabase(t *testing.T) {
	d := navDriver(t, navSchema,
		`ATTACH ':memory:' AS aux`,
		`CREATE TABLE aux.other (id INTEGER PRIMARY KEY AUTOINCREMENT, v TEXT)`,
		`CREATE INDEX aux.other_v_idx ON other (v)`,
		`CREATE VIEW aux.other_view AS SELECT id FROM other`,
		`CREATE TRIGGER aux.other_audit AFTER INSERT ON other BEGIN SELECT 1; END`,
		`INSERT INTO aux.other (v) VALUES ('x')`,
	)
	aux := navDatabase("aux")
	cases := []struct {
		folder string
		want   []string
	}{
		{"views", []string{"other_view"}},
		{"indexes", []string{"other_v_idx"}},
		{"sequences", []string{"other"}},
		{"triggers", []string{"other_audit"}},
	}
	for _, c := range cases {
		if got := childNames(navList(t, d, "database", c.folder, aux)[aux]); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("aux %s = %v, want %v", c.folder, got, c.want)
		}
	}
}

func TestListDatabaseObjects(t *testing.T) {
	d := navDriver(t, navSchema)
	main := navDatabase("main")

	if got := childNames(navList(t, d, "database", "views", main)[main]); !reflect.DeepEqual(got, []string{"parent_view"}) {
		t.Fatalf("views = %v", got)
	}

	indexes := navList(t, d, "database", "indexes", main)[main]
	idx := childNamed(t, indexes, "index", "child_parent_idx")
	if idx.Attributes["table"] != "child" || idx.Attributes["origin"] != "c" || idx.Attributes["unique"] != false {
		t.Fatalf("child_parent_idx = %+v", idx)
	}
	if idx.System {
		t.Fatalf("child_parent_idx = %+v, want not system", idx)
	}
	if auto := childNamed(t, indexes, "index", "sqlite_autoindex_parent_1"); !auto.System || auto.Attributes["origin"] != "u" || auto.Attributes["unique"] != true {
		t.Fatalf("sqlite_autoindex_parent_1 = %+v", auto)
	}

	sequences := navList(t, d, "database", "sequences", main)[main]
	if seq := childNamed(t, sequences, "sequence", "parent"); seq.Attributes["value"] != int64(2) {
		t.Fatalf("parent sequence = %+v, want value 2", seq)
	}

	triggers := navList(t, d, "database", "triggers", main)[main]
	if trig := childNamed(t, triggers, "trigger", "child_audit"); trig.Attributes["table"] != "child" {
		t.Fatalf("child_audit = %+v", trig)
	}

	types := navList(t, d, "database", "data_types", main)[main]
	if got := childNames(types); !reflect.DeepEqual(got, []string{"BLOB", "INTEGER", "NUMERIC", "REAL", "TEXT"}) {
		t.Fatalf("data types = %v", got)
	}
}

func TestListSequencesWithoutAutoincrementIsEmpty(t *testing.T) {
	d := navDriver(t, `CREATE TABLE plain (id INTEGER PRIMARY KEY)`)
	main := navDatabase("main")
	out := navList(t, d, "database", "sequences", main)
	if children, ok := out[main]; !ok || len(children) != 0 {
		t.Fatalf("sequences = %+v, want an empty listing for main", out)
	}
}

func TestListSequencesToleratesHandEditedRows(t *testing.T) {
	d := navDriver(t, navSchema, `INSERT INTO sqlite_sequence (name, seq) VALUES (NULL, 9), ('parent', 7)`)
	main := navDatabase("main")
	sequences := navList(t, d, "database", "sequences", main)[main]
	if got := childNames(sequences); !reflect.DeepEqual(got, []string{"parent"}) {
		t.Fatalf("sequences = %v, want one parent row", got)
	}
	if seq := childNamed(t, sequences, "sequence", "parent"); seq.Attributes["value"] != int64(7) {
		t.Fatalf("parent sequence = %+v, want value 7", seq)
	}
}

func TestListColumnsSkipsBrokenViewsWithoutFailingTheBatch(t *testing.T) {
	d := navDriver(t, navSchema, `CREATE TABLE gone (id INT)`, `CREATE VIEW broken AS SELECT id FROM gone`, `DROP TABLE gone`)
	good := navRelation("main", "view", "parent_view")
	broken := navRelation("main", "view", "broken")
	out := navList(t, d, "view", "columns", good, broken)
	if got := childNames(out[good]); !reflect.DeepEqual(got, []string{"id", "code"}) {
		t.Fatalf("parent_view columns = %v", got)
	}
	if children, ok := out[broken]; !ok || len(children) != 0 {
		t.Fatalf("broken view columns = %+v, want an empty listing", out)
	}
}

func TestListColumns(t *testing.T) {
	d := navDriver(t, navSchema)
	child := navRelation("main", "table", "child")
	view := navRelation("main", "view", "parent_view")
	out := navList(t, d, "table", "columns", child)
	columns := out[child]
	if got := childNames(columns); !reflect.DeepEqual(got, []string{"id", "parent_id", "doubled"}) {
		t.Fatalf("child columns = %v", got)
	}
	fk := childNamed(t, columns, "column", "parent_id")
	if fk.Attributes["foreign_key"] != true || fk.Attributes["nullable"] != true || fk.Attributes["ordinal"] != 2 || fk.Attributes["data_type"] != "INT" {
		t.Fatalf("parent_id = %+v", fk)
	}
	if doubled := childNamed(t, columns, "column", "doubled"); doubled.Attributes["generated"] != "virtual" {
		t.Fatalf("doubled = %+v, want virtual generated", doubled)
	}

	parent := navRelation("main", "table", "parent")
	id := childNamed(t, navList(t, d, "table", "columns", parent)[parent], "column", "id")
	if id.Attributes["primary_key"] != true || id.Attributes["nullable"] != false {
		t.Fatalf("parent.id = %+v", id)
	}

	viewColumns := navList(t, d, "view", "columns", view)[view]
	if got := childNames(viewColumns); !reflect.DeepEqual(got, []string{"id", "code"}) {
		t.Fatalf("view columns = %v", got)
	}

	notes := navRelation("main", "table", "notes")
	if got := childNames(navList(t, d, "table", "columns", notes)[notes]); !reflect.DeepEqual(got, []string{"body"}) {
		t.Fatalf("virtual table columns = %v, want hidden columns skipped", got)
	}
}

func TestListKeys(t *testing.T) {
	d := navDriver(t, navSchema)
	parent := navRelation("main", "table", "parent")
	pair := navRelation("main", "table", "pair")
	out := navList(t, d, "table", "keys", parent, pair)

	keys := out[parent]
	if got := childNames(keys); !reflect.DeepEqual(got, []string{"PRIMARY", "sqlite_autoindex_parent_1", "sqlite_autoindex_parent_2"}) {
		t.Fatalf("parent keys = %v", got)
	}
	if pk := childNamed(t, keys, "constraint", "PRIMARY"); pk.Attributes["constraint_type"] != "primary_key" || pk.Attributes["columns"] != "id" {
		t.Fatalf("parent PRIMARY = %+v", pk)
	}
	if uq := childNamed(t, keys, "constraint", "sqlite_autoindex_parent_2"); uq.Attributes["constraint_type"] != "unique" || uq.Attributes["columns"] != "label, code" {
		t.Fatalf("parent composite unique = %+v", uq)
	}

	if pk := childNamed(t, out[pair], "constraint", "PRIMARY"); pk.Attributes["columns"] != "b, a" {
		t.Fatalf("pair PRIMARY = %+v, want key order b, a", pk)
	}
}

func TestListForeignKeysAndReferences(t *testing.T) {
	d := navDriver(t, navSchema)
	child := navRelation("main", "table", "child")
	pair := navRelation("main", "table", "pair")
	fks := navList(t, d, "table", "foreign_keys", child, pair)

	fk := childNamed(t, fks[child], "foreign_key", "fk_0")
	if fk.Attributes["referenced_table"] != "Parent" || fk.Attributes["columns"] != "parent_id" || fk.Attributes["on_delete"] != "CASCADE" {
		t.Fatalf("child fk_0 = %+v", fk)
	}
	if composite := childNamed(t, fks[pair], "foreign_key", "fk_0"); composite.Attributes["columns"] != "a, b" {
		t.Fatalf("pair fk_0 = %+v, want columns a, b", composite)
	}

	parent := navRelation("main", "table", "parent")
	target := navRelation("main", "table", "pair_target")
	refs := navList(t, d, "table", "references", parent, target)
	if ref := childNamed(t, refs[parent], "reference", "child.fk_0"); ref.Attributes["source_table"] != "child" {
		t.Fatalf("parent reference = %+v", ref)
	}
	if got := childNames(refs[target]); !reflect.DeepEqual(got, []string{"pair.fk_0"}) {
		t.Fatalf("pair_target references = %v, want one per composite key", got)
	}
}

func TestListTableIndexesAndTriggers(t *testing.T) {
	d := navDriver(t, navSchema)
	child := navRelation("main", "table", "child")
	pair := navRelation("main", "table", "pair")

	indexes := navList(t, d, "table", "indexes", child, pair)
	if idx := childNamed(t, indexes[child], "index", "child_parent_idx"); idx.Attributes["primary"] != false || idx.Attributes["origin"] != "c" {
		t.Fatalf("child_parent_idx = %+v", idx)
	}
	if pk := childNamed(t, indexes[pair], "index", "sqlite_autoindex_pair_1"); pk.Attributes["primary"] != true || pk.Attributes["unique"] != true {
		t.Fatalf("pair primary index = %+v", pk)
	}

	triggers := navList(t, d, "table", "triggers", child, pair)
	if got := childNames(triggers[child]); !reflect.DeepEqual(got, []string{"child_audit"}) {
		t.Fatalf("child triggers = %v", got)
	}
	if len(triggers[pair]) != 0 {
		t.Fatalf("pair triggers = %v, want none", triggers[pair])
	}
}

func TestRelationFoldersMatchParentsCaseInsensitively(t *testing.T) {
	d := navDriver(t, navSchema)
	upper := navRelation("main", "table", "CHILD")
	out := navList(t, d, "table", "triggers", upper)
	if got := childNames(out[upper]); !reflect.DeepEqual(got, []string{"child_audit"}) {
		t.Fatalf("CHILD triggers = %v", got)
	}
	cols := navList(t, d, "table", "columns", upper)
	if len(cols[upper]) != 3 {
		t.Fatalf("CHILD columns = %v", cols[upper])
	}
}

func TestRelationFoldersQuoteDatabaseNames(t *testing.T) {
	d := navDriver(t, `ATTACH ':memory:' AS "odd ""name"`, `CREATE TABLE "odd ""name".t (id INT)`,
		`CREATE TRIGGER "odd ""name".t_audit AFTER INSERT ON t BEGIN SELECT 1; END`)
	db := navDatabase(`odd "name`)
	if got := childNames(navList(t, d, "database", "triggers", db)[db]); !reflect.DeepEqual(got, []string{"t_audit"}) {
		t.Fatalf("database triggers = %v", got)
	}
	table := navRelation(`odd "name`, "table", "t")
	if got := childNames(navList(t, d, "table", "triggers", table)[table]); !reflect.DeepEqual(got, []string{"t_audit"}) {
		t.Fatalf("table triggers = %v", got)
	}
}

func TestInspectDefinitionReturnsIndexDDL(t *testing.T) {
	d := navDriver(t, navSchema)
	scope := navDatabase("main")
	desc, err := d.InspectDefinition(context.Background(), metadata.ObjectRef{Scope: scope, Kind: "index", Name: "child_parent_idx"})
	if err != nil {
		t.Fatal(err)
	}
	if desc == nil || desc.Source == nil || desc.Source.Body != "CREATE INDEX child_parent_idx ON child (parent_id)" {
		t.Fatalf("index definition = %+v", desc)
	}
	auto, err := d.InspectDefinition(context.Background(), metadata.ObjectRef{Scope: scope, Kind: "index", Name: "sqlite_autoindex_parent_1"})
	if err != nil || auto != nil {
		t.Fatalf("autoindex definition = %+v, %v; want nil", auto, err)
	}
}

func TestNavigatorContract(t *testing.T) {
	d := navDriver(t, navSchema)
	main := navDatabase("main")
	child := navRelation("main", "table", "child")
	parent := navRelation("main", "table", "parent")
	pair := navRelation("main", "table", "pair")
	target := navRelation("main", "table", "pair_target")
	view := navRelation("main", "view", "parent_view")
	relations := []metadata.ScopePath{child, parent, pair, target}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "view", Folder: "columns", Parents: []metadata.ScopePath{view}},
	}
	for _, f := range []string{"tables", "views", "indexes", "sequences", "triggers", "data_types"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "database", Folder: f, Parents: []metadata.ScopePath{main}})
	}
	for _, f := range []string{"columns", "keys", "foreign_keys", "indexes", "references", "triggers"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: f, Parents: relations})
	}
	enginetest.RunNavigatorContract(t, d, "main", cases)
}
