//go:build integration

package oracle

import (
	"context"
	"database/sql"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func navRelation(kind, name string) metadata.ScopePath {
	return itScope().Child(metadata.ScopeSegment{Kind: kind, Name: name})
}

func navList(t *testing.T, d *oracleDriver, nodeKind, folderKind string, parents ...metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
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

func hasChild(children []metadata.Child, name string) bool {
	return slices.ContainsFunc(children, func(c metadata.Child) bool { return c.Name == name })
}

// navFixture creates one of every object kind the navigator lists in the
// WARDEN schema. The DDL schema trigger is created last so it does not fire
// on the rest of the fixture.
func navFixture(t *testing.T, d *oracleDriver) {
	t.Helper()
	t.Cleanup(func() {
		dropQuietly(d,
			"DROP TRIGGER nav_ddl_trg",
			"DROP SYNONYM nav_syn",
			"DROP FUNCTION nav_fn",
			"DROP PROCEDURE nav_proc",
			"DROP PACKAGE nav_pkg",
			"DROP TYPE nav_type",
			"DROP SEQUENCE nav_seq",
			"DROP MATERIALIZED VIEW nav_mv",
			"DROP VIEW nav_v",
			"DROP TABLE nav_ident PURGE",
			"DROP TABLE nav_child CASCADE CONSTRAINTS PURGE",
			"DROP TABLE nav_parent CASCADE CONSTRAINTS PURGE",
		)
	})
	for _, stmt := range []string{
		`CREATE TABLE nav_parent (id NUMBER CONSTRAINT nav_parent_pk PRIMARY KEY, code VARCHAR2(20) CONSTRAINT nav_parent_uq UNIQUE)`,
		`CREATE TABLE nav_child (
			id NUMBER CONSTRAINT nav_child_pk PRIMARY KEY,
			parent_id NUMBER CONSTRAINT nav_child_fk REFERENCES nav_parent(id) ON DELETE CASCADE,
			qty NUMBER(8,2) NOT NULL CONSTRAINT nav_child_chk CHECK (qty > 0),
			note CLOB
		)`,
		`CREATE INDEX nav_child_qty_idx ON nav_child (qty)`,
		`CREATE TABLE nav_ident (id NUMBER GENERATED ALWAYS AS IDENTITY)`,
		`CREATE VIEW nav_v AS SELECT c.id, p.code FROM nav_child c JOIN nav_parent p ON p.id = c.parent_id WITH READ ONLY`,
		`CREATE MATERIALIZED VIEW nav_mv AS SELECT id, code FROM nav_parent`,
		`CREATE INDEX nav_mv_code_idx ON nav_mv (code)`,
		`CREATE SEQUENCE nav_seq`,
		`CREATE OR REPLACE TYPE nav_type AS OBJECT (id NUMBER)`,
		`CREATE OR REPLACE PACKAGE nav_pkg AS last_parent nav_parent%ROWTYPE; FUNCTION answer RETURN NUMBER; END;`,
		`CREATE OR REPLACE PACKAGE BODY nav_pkg AS FUNCTION answer RETURN NUMBER AS n NUMBER; BEGIN SELECT COUNT(*) INTO n FROM nav_parent; RETURN n; END; END;`,
		`CREATE OR REPLACE PROCEDURE nav_proc AS BEGIN NULL; END;`,
		`CREATE OR REPLACE FUNCTION nav_fn RETURN NUMBER AS BEGIN RETURN 1; END;`,
		`CREATE SYNONYM nav_syn FOR nav_parent`,
		`CREATE OR REPLACE TRIGGER nav_child_trg BEFORE INSERT ON nav_child FOR EACH ROW BEGIN :NEW.qty := NVL(:NEW.qty, 1); END;`,
		`CREATE OR REPLACE TRIGGER nav_ddl_trg AFTER DDL ON SCHEMA BEGIN NULL; END;`,
	} {
		mustExec(t, d, stmt)
	}
}

func TestOracleNavigator(t *testing.T) {
	d := newConnectedDriver(t)
	navFixture(t, d)
	schema := itScope()
	parent := navRelation("table", "NAV_PARENT")
	child := navRelation("table", "NAV_CHILD")
	view := navRelation("view", "NAV_V")
	mview := navRelation("materialized_view", "NAV_MV")

	t.Run("contract", func(t *testing.T) {
		enginetest.RunNavigatorContract(t, d, oracleITSchema, []enginetest.NavigatorCase{
			{NodeKind: "", Folder: "schemas", Parents: []metadata.ScopePath{""}},
			{NodeKind: "", Folder: "types", Parents: []metadata.ScopePath{""}},
			{NodeKind: "", Folder: "users", Parents: []metadata.ScopePath{""}},
			{NodeKind: "", Folder: "roles", Parents: []metadata.ScopePath{""}},
			{NodeKind: "", Folder: "profiles", Parents: []metadata.ScopePath{""}},
			{NodeKind: "schema", Folder: "tables", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "views", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "materialized_views", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "indexes", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "sequences", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "queues", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "types", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "packages", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "procedures", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "functions", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "synonyms", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "schema_triggers", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "schema", Folder: "table_triggers", Parents: []metadata.ScopePath{schema}},
			{NodeKind: "table", Folder: "columns", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "constraints", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "foreign_keys", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "references", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "triggers", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "indexes", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "table", Folder: "dependencies", Parents: []metadata.ScopePath{parent, child}},
			{NodeKind: "view", Folder: "columns", Parents: []metadata.ScopePath{view}},
			{NodeKind: "view", Folder: "constraints", Parents: []metadata.ScopePath{view}},
			{NodeKind: "view", Folder: "dependencies", Parents: []metadata.ScopePath{view}},
			{NodeKind: "materialized_view", Folder: "columns", Parents: []metadata.ScopePath{mview}},
			{NodeKind: "materialized_view", Folder: "indexes", Parents: []metadata.ScopePath{mview}},
			{NodeKind: "materialized_view", Folder: "dependencies", Parents: []metadata.ScopePath{mview}},
		})
	})

	t.Run("schemas mark current and system", func(t *testing.T) {
		schemas := navList(t, d, "", "schemas", "")[""]
		if got := childNamed(t, schemas, "schema", oracleITSchema); !got.Current || got.System {
			t.Fatalf("WARDEN = %+v, want current non-system", got)
		}
		if got := childNamed(t, schemas, "schema", "SYS"); !got.System || got.Current {
			t.Fatalf("SYS = %+v, want system non-current", got)
		}
	})

	t.Run("root folders", func(t *testing.T) {
		users := navList(t, d, "", "users", "")[""]
		if got := childNamed(t, users, "user", oracleITSchema); !got.Current {
			t.Fatalf("WARDEN user = %+v, want current", got)
		}
		if got := childNamed(t, users, "user", "SYS"); !got.System {
			t.Fatalf("SYS user = %+v, want system", got)
		}
		if got := childNamed(t, navList(t, d, "", "roles", "")[""], "role", "DBA"); !got.System {
			t.Fatalf("DBA role = %+v, want system", got)
		}
		childNamed(t, navList(t, d, "", "profiles", "")[""], "profile", "DEFAULT")
	})

	t.Run("tables exclude storage tables", func(t *testing.T) {
		tables := navList(t, d, "schema", "tables", schema)[schema]
		for _, name := range []string{"NAV_PARENT", "NAV_CHILD", "NAV_IDENT"} {
			childNamed(t, tables, "table", name)
		}
		if hasChild(tables, "NAV_MV") {
			t.Fatalf("materialized view container listed as a table: %+v", tables)
		}
		childNamed(t, navList(t, d, "schema", "materialized_views", schema)[schema], "materialized_view", "NAV_MV")
		childNamed(t, navList(t, d, "schema", "views", schema)[schema], "view", "NAV_V")
	})

	t.Run("schema objects", func(t *testing.T) {
		for _, c := range []struct{ folder, kind, name string }{
			{"sequences", "sequence", "NAV_SEQ"},
			{"types", "type", "NAV_TYPE"},
			{"packages", "package", "NAV_PKG"},
			{"procedures", "procedure", "NAV_PROC"},
			{"functions", "function", "NAV_FN"},
			{"synonyms", "synonym", "NAV_SYN"},
			{"schema_triggers", "trigger", "NAV_DDL_TRG"},
			{"table_triggers", "trigger", "NAV_CHILD_TRG"},
			{"indexes", "index", "NAV_CHILD_QTY_IDX"},
		} {
			childNamed(t, navList(t, d, "schema", c.folder, schema)[schema], c.kind, c.name)
		}
		if got := childNamed(t, navList(t, d, "schema", "synonyms", schema)[schema], "synonym", "NAV_SYN"); got.Attributes["target"] != oracleITSchema+".NAV_PARENT" {
			t.Fatalf("synonym target = %v", got.Attributes["target"])
		}
		if got := childNamed(t, navList(t, d, "schema", "table_triggers", schema)[schema], "trigger", "NAV_CHILD_TRG"); got.Attributes["table"] != "NAV_CHILD" {
			t.Fatalf("trigger table = %v", got.Attributes["table"])
		}
		if hasChild(navList(t, d, "schema", "table_triggers", schema)[schema], "NAV_DDL_TRG") {
			t.Fatal("schema trigger listed under table triggers")
		}
		sequences := navList(t, d, "schema", "sequences", schema)[schema]
		for _, s := range sequences {
			if s.Name != "NAV_SEQ" && !s.System && len(s.Name) > 7 && s.Name[:7] == "ISEQ$$_" {
				t.Fatalf("identity sequence %q not flagged system", s.Name)
			}
		}
		for _, i := range navList(t, d, "schema", "indexes", schema)[schema] {
			if i.Attributes["method"] == "lob" && !i.System {
				t.Fatalf("LOB index %q not flagged system", i.Name)
			}
		}
	})

	t.Run("columns", func(t *testing.T) {
		columns := navList(t, d, "table", "columns", child)[child]
		want := map[string]any{"data_type": "NUMBER(8,2)", "nullable": false, "ordinal": int64(3), "primary_key": false, "foreign_key": false}
		if got := childNamed(t, columns, "column", "QTY"); !reflect.DeepEqual(got.Attributes, want) {
			t.Fatalf("QTY = %+v, want %+v", got.Attributes, want)
		}
		if got := childNamed(t, columns, "column", "ID"); got.Attributes["primary_key"] != true {
			t.Fatalf("ID = %+v, want primary key", got.Attributes)
		}
		if got := childNamed(t, columns, "column", "PARENT_ID"); got.Attributes["foreign_key"] != true {
			t.Fatalf("PARENT_ID = %+v, want foreign key", got.Attributes)
		}
		childNamed(t, navList(t, d, "view", "columns", view)[view], "column", "CODE")
	})

	t.Run("constraints flag generated not null checks", func(t *testing.T) {
		constraints := navList(t, d, "table", "constraints", child)[child]
		if got := childNamed(t, constraints, "constraint", "NAV_CHILD_CHK"); got.System || got.Attributes["constraint_type"] != "check" {
			t.Fatalf("NAV_CHILD_CHK = %+v", got)
		}
		if got := childNamed(t, constraints, "constraint", "NAV_CHILD_PK"); got.Attributes["constraint_type"] != "primary_key" {
			t.Fatalf("NAV_CHILD_PK = %+v", got)
		}
		notNull := 0
		for _, c := range constraints {
			if c.System {
				notNull++
			}
		}
		if notNull != 1 {
			t.Fatalf("want exactly one system NOT NULL check, got %+v", constraints)
		}
		if got := navList(t, d, "view", "constraints", view)[view]; len(got) != 1 || got[0].Attributes["constraint_type"] != "read_only" {
			t.Fatalf("view constraints = %+v, want one read_only", got)
		}
	})

	t.Run("keys and references", func(t *testing.T) {
		fk := childNamed(t, navList(t, d, "table", "foreign_keys", child)[child], "foreign_key", "NAV_CHILD_FK")
		if fk.Attributes["referenced_table"] != oracleITSchema+".NAV_PARENT" || fk.Attributes["delete_rule"] != "cascade" {
			t.Fatalf("NAV_CHILD_FK = %+v", fk)
		}
		ref := childNamed(t, navList(t, d, "table", "references", parent)[parent], "reference", "NAV_CHILD_FK")
		if ref.Attributes["source_table"] != oracleITSchema+".NAV_CHILD" {
			t.Fatalf("reference = %+v", ref)
		}
	})

	t.Run("relation triggers and indexes", func(t *testing.T) {
		trg := childNamed(t, navList(t, d, "table", "triggers", child)[child], "trigger", "NAV_CHILD_TRG")
		if trg.Attributes["event"] != "insert" {
			t.Fatalf("trigger = %+v", trg)
		}
		indexes := navList(t, d, "table", "indexes", child)[child]
		if got := childNamed(t, indexes, "index", "NAV_CHILD_QTY_IDX"); got.Attributes["primary"] != false || got.Attributes["unique"] != false {
			t.Fatalf("NAV_CHILD_QTY_IDX = %+v", got)
		}
		pk := false
		for _, i := range indexes {
			pk = pk || i.Attributes["primary"] == true
		}
		if !pk {
			t.Fatalf("no primary key index in %+v", indexes)
		}
		childNamed(t, navList(t, d, "materialized_view", "indexes", mview)[mview], "index", "NAV_MV_CODE_IDX")
	})

	t.Run("dependencies both directions", func(t *testing.T) {
		deps := navList(t, d, "table", "dependencies", parent)[parent]
		for _, name := range []string{"NAV_V", "NAV_MV", "NAV_PKG"} {
			got := childNamed(t, deps, "dependency", oracleITSchema+"."+name)
			if got.Attributes["direction"] != "dependent" {
				t.Fatalf("%s = %+v, want dependent", name, got)
			}
		}
		seen := map[string]bool{}
		for _, dep := range deps {
			if seen[dep.Name] {
				t.Fatalf("dependency %q listed twice: %+v", dep.Name, deps)
			}
			seen[dep.Name] = true
		}
		if got := childNamed(t, deps, "dependency", oracleITSchema+".NAV_PKG"); got.Attributes["object_kind"] != "package" {
			t.Fatalf("NAV_PKG = %+v, want package", got)
		}
		got := childNamed(t, navList(t, d, "view", "dependencies", view)[view], "dependency", oracleITSchema+".NAV_CHILD")
		if got.Attributes["direction"] != "dependency" || got.Attributes["object_kind"] != "table" {
			t.Fatalf("view dependency = %+v", got)
		}
		for _, dep := range navList(t, d, "materialized_view", "dependencies", mview)[mview] {
			if dep.Name == oracleITSchema+".NAV_MV" {
				t.Fatalf("materialized view lists itself: %+v", dep)
			}
		}
	})

	t.Run("definitions", func(t *testing.T) {
		for _, ref := range []metadata.ObjectRef{
			{Scope: schema, Kind: "package", Name: "NAV_PKG"},
			{Scope: schema, Kind: "procedure", Name: "NAV_PROC"},
			{Scope: schema, Kind: "function", Name: "NAV_FN"},
			{Scope: schema, Kind: "trigger", Name: "NAV_CHILD_TRG"},
			{Scope: schema, Kind: "type", Name: "NAV_TYPE"},
			{Scope: schema, Kind: "foreign_key", Name: "NAV_CHILD_FK"},
		} {
			desc, err := d.InspectDefinition(context.Background(), ref)
			if err != nil || desc == nil {
				t.Fatalf("InspectDefinition(%s %s) = %v, %v", ref.Kind, ref.Name, desc, err)
			}
		}
	})

	t.Run("other owners", func(t *testing.T) {
		admin := navAdmin(t)
		navLimitedAccount(t, admin, "GRANT UNLIMITED TABLESPACE TO NAV_LIMITED")
		mustExec(t, d, "GRANT REFERENCES ON nav_parent TO NAV_LIMITED")
		mustExec(t, d, "GRANT EXECUTE ON nav_fn TO NAV_LIMITED")
		mustExec(t, d, "GRANT EXECUTE ON nav_pkg TO NAV_LIMITED")
		for _, stmt := range []string{
			"CREATE TABLE NAV_LIMITED.nav_ext (pid NUMBER CONSTRAINT nav_child_fk REFERENCES " + oracleITSchema + ".nav_parent(id))",
			"CREATE INDEX NAV_LIMITED.nav_child_qty_idx ON " + oracleITSchema + ".nav_child (parent_id)",
			"CREATE TRIGGER NAV_LIMITED.nav_child_trg BEFORE UPDATE ON " + oracleITSchema + ".nav_child BEGIN NULL; END;",
		} {
			if _, err := admin.ExecContext(context.Background(), stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		sys := navConnect(t, "system", "warden_sys")

		refs := navList(t, sys, "table", "references", parent)[parent]
		childNamed(t, refs, "reference", "NAV_CHILD_FK")
		childNamed(t, refs, "reference", "NAV_LIMITED.NAV_CHILD_FK")
		triggers := navList(t, sys, "table", "triggers", child)[child]
		childNamed(t, triggers, "trigger", "NAV_CHILD_TRG")
		childNamed(t, triggers, "trigger", "NAV_LIMITED.NAV_CHILD_TRG")
		indexes := navList(t, sys, "table", "indexes", child)[child]
		childNamed(t, indexes, "index", "NAV_CHILD_QTY_IDX")
		childNamed(t, indexes, "index", "NAV_LIMITED.NAV_CHILD_QTY_IDX")

		for _, ref := range []metadata.ObjectRef{
			{Scope: schema, Kind: "index", Name: "NAV_LIMITED.NAV_CHILD_QTY_IDX"},
			{Scope: schema, Kind: "trigger", Name: "NAV_LIMITED.NAV_CHILD_TRG"},
		} {
			desc, err := sys.InspectDefinition(context.Background(), ref)
			if err != nil || desc == nil || !strings.Contains(desc.Source.Body, "NAV_LIMITED") {
				t.Fatalf("InspectDefinition(%s %s) = %+v, %v", ref.Kind, ref.Name, desc, err)
			}
		}

		limited := navConnect(t, "NAV_LIMITED", "nav_test")
		for _, ref := range []metadata.ObjectRef{
			{Scope: schema, Kind: "function", Name: "NAV_FN"},
			{Scope: schema, Kind: "package", Name: "NAV_PKG"},
		} {
			desc, err := limited.InspectDefinition(context.Background(), ref)
			if err != nil || desc == nil || !strings.Contains(desc.Source.Body, "CREATE OR REPLACE") {
				t.Fatalf("EXECUTE-only InspectDefinition(%s %s) = %+v, %v", ref.Kind, ref.Name, desc, err)
			}
		}

		if got := childNamed(t, navList(t, sys, "", "schemas", "")[""], "schema", "SYSTEM"); !got.Current || got.System {
			t.Fatalf("SYSTEM = %+v, want current non-system when logged in as SYSTEM", got)
		}
	})
}

func navAdmin(t *testing.T) *sql.DB {
	t.Helper()
	adminURL, err := url.Parse(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	adminURL.User = url.UserPassword("system", "warden_sys")
	admin, err := sql.Open("oracle", adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	return admin
}

// navLimitedAccount creates NAV_LIMITED with only CREATE SESSION plus grants,
// dropped with everything it owns when the test ends.
func navLimitedAccount(t *testing.T, admin *sql.DB, grants ...string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range append([]string{"CREATE USER NAV_LIMITED IDENTIFIED BY nav_test", "GRANT CREATE SESSION TO NAV_LIMITED"}, grants...) {
		if _, err := admin.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() { _, _ = admin.ExecContext(ctx, "DROP USER NAV_LIMITED CASCADE") })
}

func navConnect(t *testing.T, user, password string) *oracleDriver {
	t.Helper()
	loginURL, err := url.Parse(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	loginURL.User = url.UserPassword(user, password)
	d := &oracleDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: loginURL.String(), Driver: "oracle"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOracleNavigatorRestrictedAccount(t *testing.T) {
	navLimitedAccount(t, navAdmin(t))
	d := navConnect(t, "NAV_LIMITED", "nav_test")
	for _, folder := range []string{"roles", "profiles"} {
		if got := navList(t, d, "", folder, "")[""]; len(got) != 0 {
			t.Fatalf("%s = %+v, want empty listing", folder, got)
		}
	}
	if got := childNamed(t, navList(t, d, "", "schemas", "")[""], "schema", "NAV_LIMITED"); !got.Current {
		t.Fatalf("NAV_LIMITED = %+v, want current", got)
	}
}
