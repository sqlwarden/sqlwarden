package sqlserver

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completioncore/mssql"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

const navDatabase = "nav_db"

func TestNavigatorTreeValid(t *testing.T) {
	if err := navigatorTree.Validate(); err != nil {
		t.Fatal(err)
	}
	if kind := navigatorTree.DatabaseKind(); kind != "database" {
		t.Fatalf("DatabaseKind() = %q, want database", kind)
	}
}

func TestMergeIndexes(t *testing.T) {
	idx := func(name, table string, system bool) metadata.Child {
		return metadata.Child{Kind: "index", Name: name, System: system, Attributes: map[string]any{"table": table}}
	}
	got := mergeIndexes([]metadata.Child{idx("a", "t1", false), idx("a", "t2", true), idx("b", "t1", true), idx("b", "t2", true)})
	want := []metadata.Child{idx("a", "t1, t2", false), idx("b", "t1, t2", true)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeIndexes = %+v, want %+v", got, want)
	}
	got = mergeIndexes([]metadata.Child{idx("IDX", "a", true), idx("idx", "b", true), idx("IDX", "c", false)})
	want = []metadata.Child{idx("IDX", "a, c", false), idx("idx", "b", true)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeIndexes non-adjacent = %+v, want %+v", got, want)
	}
}

func TestValuesList(t *testing.T) {
	values, args := valuesList([][]string{{"s", "a"}, {"s", "b"}})
	if values != "(@p1,@p2),(@p3,@p4)" {
		t.Fatalf("values = %q", values)
	}
	if !reflect.DeepEqual(args, []any{"s", "a", "s", "b"}) {
		t.Fatalf("args = %v", args)
	}
}

func navScope(segments ...string) metadata.ScopePath {
	path := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: navDatabase})
	for i := 0; i+1 < len(segments); i += 2 {
		path = path.Child(metadata.ScopeSegment{Kind: segments[i], Name: segments[i+1]})
	}
	return path
}

func navList(t *testing.T, d *Driver, database, nodeKind, folderKind string, parents ...metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	t.Helper()
	folder, ok := d.Tree().Folder(nodeKind, folderKind)
	if !ok {
		t.Fatalf("folder %q under %q not declared", folderKind, nodeKind)
	}
	q, err := d.Querier(context.Background(), database)
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

func hasChild(children []metadata.Child, name string) bool {
	return slices.ContainsFunc(children, func(c metadata.Child) bool { return c.Name == name })
}

func mustExec(t *testing.T, d *Driver, stmt string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), stmt); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// navFixture creates nav_db with one of every object kind the navigator
// lists, through a second driver whose default database is nav_db. The
// database trigger is created last so it does not fire on the rest.
func navFixture(t *testing.T, d *Driver) {
	t.Helper()
	mustExec(t, d, "CREATE DATABASE "+navDatabase)
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), "ALTER DATABASE "+navDatabase+" SET SINGLE_USER WITH ROLLBACK IMMEDIATE")
		_, _ = d.Execute(context.Background(), "DROP DATABASE "+navDatabase)
	})
	nav := &Driver{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := nav.Connect(ctx, engine.ConnectionConfig{
		DSN: testDSN, Driver: "sqlserver", DefaultScope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: navDatabase}),
	}); err != nil {
		t.Fatalf("Connect %s: %v", navDatabase, err)
	}
	defer nav.Close()
	for _, stmt := range []string{
		`CREATE SCHEMA nav`,
		`CREATE TABLE nav.parent (id INT CONSTRAINT nav_parent_pk PRIMARY KEY, code NVARCHAR(20) CONSTRAINT nav_parent_uq UNIQUE)`,
		`CREATE TABLE nav.child (
			id INT IDENTITY(1,1) CONSTRAINT nav_child_pk PRIMARY KEY,
			parent_id INT CONSTRAINT nav_child_fk REFERENCES nav.parent(id) ON DELETE CASCADE,
			qty DECIMAL(8,2) NOT NULL CONSTRAINT nav_child_chk CHECK (qty > 0),
			total AS qty * 2
		)`,
		`CREATE TABLE dbo.outside (id INT, parent_id INT CONSTRAINT nav_outside_fk REFERENCES nav.parent(id))`,
		`INSERT INTO nav.parent (id, code) VALUES (1, 'a'), (2, 'b')`,
		`CREATE INDEX shared_idx ON nav.child (qty)`,
		`CREATE INDEX shared_idx ON nav.parent (code)`,
		`CREATE VIEW nav.v AS SELECT c.id, p.code FROM nav.child c JOIN nav.parent p ON p.id = c.parent_id`,
		`CREATE TRIGGER nav.v_trg ON nav.v INSTEAD OF INSERT AS BEGIN SET NOCOUNT ON; END`,
		`CREATE TRIGGER nav.child_trg ON nav.child AFTER INSERT AS BEGIN SET NOCOUNT ON; END`,
		`CREATE PROCEDURE nav.proc1 AS SELECT 1`,
		`CREATE FUNCTION nav.fn1() RETURNS INT AS BEGIN RETURN 1 END`,
		`CREATE FUNCTION nav.tvf1() RETURNS TABLE AS RETURN SELECT 1 AS id`,
		`CREATE SEQUENCE nav.seq1 AS BIGINT`,
		`CREATE SYNONYM nav.syn1 FOR nav.parent`,
		`CREATE TYPE nav.code_t FROM NVARCHAR(20)`,
		`CREATE TYPE nav.rows_t AS TABLE (id INT)`,
		`EXEC sp_addextendedproperty N'MS_Description', N'parents', N'SCHEMA', N'nav', N'TABLE', N'parent'`,
		`CREATE TRIGGER nav_ddl_trg ON DATABASE FOR CREATE_TABLE AS BEGIN SET NOCOUNT ON; END`,
	} {
		mustExec(t, nav, stmt)
	}
}

func TestSQLServerNavigator(t *testing.T) {
	d := newConnectedDriver(t)
	navFixture(t, d)
	ctx := context.Background()
	database := navScope()
	schema := navScope("schema", "nav")
	parent := navScope("schema", "nav", "table", "parent")
	child := navScope("schema", "nav", "table", "child")
	view := navScope("schema", "nav", "view", "v")

	t.Run("contract", func(t *testing.T) {
		enginetest.RunNavigatorContract(t, d, "", []enginetest.NavigatorCase{
			{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
			{NodeKind: "", Folder: "logins", Parents: []metadata.ScopePath{""}},
		})
		schemas := []metadata.ScopePath{schema, navScope("schema", "dbo")}
		enginetest.RunNavigatorContract(t, d, navDatabase, []enginetest.NavigatorCase{
			{NodeKind: "database", Folder: "schemas", Parents: []metadata.ScopePath{database}},
			{NodeKind: "database", Folder: "database_triggers", Parents: []metadata.ScopePath{database}},
			{NodeKind: "schema", Folder: "tables", Parents: schemas},
			{NodeKind: "schema", Folder: "external_tables", Parents: schemas},
			{NodeKind: "schema", Folder: "views", Parents: schemas},
			{NodeKind: "schema", Folder: "indexes", Parents: schemas},
			{NodeKind: "schema", Folder: "procedures", Parents: schemas},
			{NodeKind: "schema", Folder: "sequences", Parents: schemas},
			{NodeKind: "schema", Folder: "synonyms", Parents: schemas},
			{NodeKind: "schema", Folder: "triggers", Parents: schemas},
			{NodeKind: "schema", Folder: "data_types", Parents: schemas},
			{NodeKind: "table", Folder: "columns", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "unique_keys", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "check_constraints", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "foreign_keys", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "indexes", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "references", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "triggers", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "extended_properties", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "view", Folder: "columns", Parents: []metadata.ScopePath{view}},
			{NodeKind: "view", Folder: "triggers", Parents: []metadata.ScopePath{view}},
			{NodeKind: "view", Folder: "extended_properties", Parents: []metadata.ScopePath{view}},
		})
	})

	t.Run("root", func(t *testing.T) {
		databases := navList(t, d, "", "", "databases", "")[""]
		if db := childNamed(t, databases, "database", navDatabase); db.System || db.Current {
			t.Fatalf("%s = %+v, want user database, not current", navDatabase, db)
		}
		if master := childNamed(t, databases, "database", "master"); !master.Current || master.System {
			t.Fatalf("master = %+v, want current and not system", master)
		}
		if !childNamed(t, databases, "database", "tempdb").System {
			t.Fatal("tempdb not flagged system")
		}
		logins := navList(t, d, "", "", "logins", "")[""]
		if sa := childNamed(t, logins, "login", "sa"); !sa.Current || sa.System {
			t.Fatalf("sa = %+v, want current and not system", sa)
		}
		if !slices.ContainsFunc(logins, func(c metadata.Child) bool { return strings.HasPrefix(c.Name, "##") && c.System }) {
			t.Fatalf("no system ## login in %+v", logins)
		}
	})

	t.Run("database", func(t *testing.T) {
		schemas := navList(t, d, navDatabase, "database", "schemas", database)[database]
		if nav := childNamed(t, schemas, "schema", "nav"); nav.System || nav.Current {
			t.Fatalf("nav = %+v", nav)
		}
		for _, name := range []string{"sys", "INFORMATION_SCHEMA", "db_owner"} {
			if !childNamed(t, schemas, "schema", name).System {
				t.Fatalf("schema %s not flagged system", name)
			}
		}
		master := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "master"})
		if dbo := childNamed(t, navList(t, d, "master", "database", "schemas", master)[master], "schema", "dbo"); !dbo.Current {
			t.Fatalf("dbo in login database = %+v, want current", dbo)
		}
		triggers := navList(t, d, navDatabase, "database", "database_triggers", database)[database]
		childNamed(t, triggers, "trigger", "nav_ddl_trg")
	})

	t.Run("schema", func(t *testing.T) {
		list := func(folder string) []metadata.Child {
			return navList(t, d, navDatabase, "schema", folder, schema)[schema]
		}
		if rows := childNamed(t, list("tables"), "table", "parent").Attributes["row_count"]; rows != int64(2) {
			t.Fatalf("parent row_count = %v, want 2", rows)
		}
		childNamed(t, list("tables"), "table", "child")
		if len(list("external_tables")) != 0 {
			t.Fatal("unexpected external tables")
		}
		childNamed(t, list("views"), "view", "v")
		if shared := childNamed(t, list("indexes"), "index", "shared_idx"); shared.Attributes["table"] != "child, parent" {
			t.Fatalf("shared_idx table = %v", shared.Attributes["table"])
		}
		routines := list("procedures")
		childNamed(t, routines, "procedure", "proc1")
		if fn := childNamed(t, routines, "function", "fn1"); fn.Attributes[mssql.ReturnsTableAttribute] != nil {
			t.Fatalf("fn1 = %+v, want a scalar function", fn)
		}
		if tvf := childNamed(t, routines, "function", "tvf1"); tvf.Attributes[mssql.ReturnsTableAttribute] != true {
			t.Fatalf("tvf1 = %+v, want returns_table", tvf)
		}
		if seq := childNamed(t, list("sequences"), "sequence", "seq1"); seq.Attributes["data_type"] != "bigint" {
			t.Fatalf("seq1 = %+v", seq)
		}
		if syn := childNamed(t, list("synonyms"), "synonym", "syn1"); syn.Attributes["target"] != "[nav].[parent]" {
			t.Fatalf("syn1 = %+v", syn)
		}
		triggers := list("triggers")
		if trg := childNamed(t, triggers, "trigger", "v_trg"); trg.Attributes["table"] != "v" || trg.Attributes["instead_of"] != true {
			t.Fatalf("v_trg = %+v", trg)
		}
		childNamed(t, triggers, "trigger", "child_trg")
		types := list("data_types")
		if code := childNamed(t, types, "type", "code_t"); code.Attributes["base_type"] != "nvarchar" {
			t.Fatalf("code_t = %+v", code)
		}
		if rows := childNamed(t, types, "type", "rows_t"); rows.Attributes["table_type"] != true {
			t.Fatalf("rows_t = %+v", rows)
		}
	})

	t.Run("table", func(t *testing.T) {
		list := func(folder string, p metadata.ScopePath) []metadata.Child {
			return navList(t, d, navDatabase, "table", folder, p)[p]
		}
		columns := list("columns", child)
		if id := childNamed(t, columns, "column", "id"); id.Attributes["primary_key"] != true || id.Attributes["identity"] != true {
			t.Fatalf("id = %+v", id)
		}
		if fk := childNamed(t, columns, "column", "parent_id"); fk.Attributes["foreign_key"] != true || fk.Attributes["nullable"] != true {
			t.Fatalf("parent_id = %+v", fk)
		}
		if qty := childNamed(t, columns, "column", "qty"); qty.Attributes["data_type"] != "decimal(8,2)" || qty.Attributes["nullable"] != false {
			t.Fatalf("qty = %+v", qty)
		}
		if total := childNamed(t, columns, "column", "total"); total.Attributes["computed"] != true {
			t.Fatalf("total = %+v", total)
		}
		keys := list("unique_keys", parent)
		if pk := childNamed(t, keys, "unique_key", "nav_parent_pk"); pk.Attributes["constraint_type"] != "primary_key" {
			t.Fatalf("nav_parent_pk = %+v", pk)
		}
		if uq := childNamed(t, keys, "unique_key", "nav_parent_uq"); uq.Attributes["constraint_type"] != "unique" {
			t.Fatalf("nav_parent_uq = %+v", uq)
		}
		childNamed(t, list("check_constraints", child), "check_constraint", "nav_child_chk")
		fk := childNamed(t, list("foreign_keys", child), "foreign_key", "nav_child_fk")
		if fk.Attributes["referenced_table"] != "nav.parent" || fk.Attributes["delete_rule"] != "cascade" {
			t.Fatalf("nav_child_fk = %+v", fk)
		}
		indexes := list("indexes", parent)
		if pk := childNamed(t, indexes, "index", "nav_parent_pk"); pk.Attributes["primary"] != true || pk.Attributes["unique"] != true {
			t.Fatalf("nav_parent_pk index = %+v", pk)
		}
		childNamed(t, indexes, "index", "shared_idx")
		refs := list("references", parent)
		childNamed(t, refs, "reference", "nav_child_fk")
		if outside := childNamed(t, refs, "reference", "dbo.nav_outside_fk"); outside.Attributes["source_table"] != "dbo.outside" {
			t.Fatalf("dbo.nav_outside_fk = %+v", outside)
		}
		childNamed(t, list("triggers", child), "trigger", "child_trg")
		if hasChild(list("triggers", parent), "child_trg") {
			t.Fatal("child_trg listed under parent")
		}
		if prop := childNamed(t, list("extended_properties", parent), "extended_property", "MS_Description"); prop.Attributes["value"] != "parents" {
			t.Fatalf("MS_Description = %+v", prop)
		}
		childNamed(t, navList(t, d, navDatabase, "view", "triggers", view)[view], "trigger", "v_trg")
	})

	t.Run("requested name case", func(t *testing.T) {
		upperSchema := navScope("schema", "NAV")
		childNamed(t, navList(t, d, navDatabase, "schema", "tables", upperSchema)[upperSchema], "table", "parent")
		upperTable := navScope("schema", "NAV", "table", "PARENT")
		childNamed(t, navList(t, d, navDatabase, "table", "columns", upperTable)[upperTable], "column", "code")
	})

	t.Run("querier does not leak database", func(t *testing.T) {
		d.db.SetMaxOpenConns(1)
		defer d.db.SetMaxOpenConns(0)
		for range 3 {
			navList(t, d, navDatabase, "database", "schemas", database)
			var current string
			if err := d.db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&current); err != nil {
				t.Fatal(err)
			}
			if current != "master" {
				t.Fatalf("session database = %q after navigator query, want master", current)
			}
		}
	})

	t.Run("inspect in database", func(t *testing.T) {
		ref := func(kind, name string, scope metadata.ScopePath) metadata.ObjectRef {
			return metadata.ObjectRef{Kind: kind, Name: name, Scope: scope}
		}
		objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref("table", "parent", schema), ref("procedure", "proc1", schema)})
		if err != nil {
			t.Fatal(err)
		}
		if len(objects) != 2 || objects[0].Relational == nil || len(objects[0].Relational.Columns) != 2 {
			t.Fatalf("objects = %+v", objects)
		}
		fkObjects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref("table", "child", schema)})
		if err != nil {
			t.Fatal(err)
		}
		if fks := fkObjects[0].Relational.ForeignKeys; len(fks) != 1 || fks[0].References != ref("table", "parent", schema) {
			t.Fatalf("child foreign keys = %+v, want target in %s", fks, schema)
		}
		for _, c := range []struct {
			ref  metadata.ObjectRef
			want string
		}{
			{ref("table", "parent", schema), "CREATE TABLE"},
			{ref("procedure", "proc1", schema), "CREATE PROCEDURE nav.proc1"},
			{ref("trigger", "child_trg", schema), "CREATE TRIGGER nav.child_trg"},
			{ref("trigger", "nav_ddl_trg", database), "CREATE TRIGGER nav_ddl_trg ON DATABASE"},
		} {
			desc, err := d.InspectDefinition(ctx, c.ref)
			if err != nil {
				t.Fatalf("%s: %v", c.ref.Name, err)
			}
			if desc == nil || !strings.Contains(desc.Source.Body, c.want) {
				t.Fatalf("%s definition = %+v, want %q", c.ref.Name, desc, c.want)
			}
		}
	})
}
