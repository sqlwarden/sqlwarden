package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

// TestPostgresInspectObjectsCoversNewKinds proves every new schema-object kind
// added for Postgres/MySQL parity (type, domain, procedure, trigger,
// partitioned tables, and foreign tables) is served by bulk object inspection
// against a live database.
func TestPostgresInspectObjectsCoversNewKinds(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyPostgres(t, d,
			"DROP FOREIGN TABLE IF EXISTS inspect_foreign_pg_test",
			"DROP SERVER IF EXISTS inspect_loopback_pg_test CASCADE",
			"DROP TABLE IF EXISTS inspect_foreign_target_pg_test",
			"DROP TABLE IF EXISTS events_p CASCADE",
			"DROP TRIGGER IF EXISTS widgets_trg ON widgets",
			"DROP FUNCTION IF EXISTS widgets_trg_fn()",
			"DROP PROCEDURE IF EXISTS noop()",
			"DROP TABLE IF EXISTS widgets",
			"DROP DOMAIN IF EXISTS positive_int",
			"DROP TYPE IF EXISTS mood",
		)
	})

	mustExec(t, d, `CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy')`)
	mustExec(t, d, `CREATE DOMAIN positive_int AS integer CHECK (VALUE > 0)`)
	mustExec(t, d, `CREATE TABLE widgets (id serial PRIMARY KEY, label text)`)
	mustExec(t, d, `CREATE PROCEDURE noop() LANGUAGE plpgsql AS $$ BEGIN END $$`)
	mustExec(t, d, `CREATE FUNCTION widgets_trg_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER widgets_trg BEFORE INSERT ON widgets FOR EACH ROW EXECUTE FUNCTION widgets_trg_fn()`)
	mustExec(t, d, `CREATE TABLE events_p (id int, created_at date) PARTITION BY RANGE (created_at)`)
	mustExec(t, d, `CREATE TABLE events_p_2026 PARTITION OF events_p FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)

	mustExec(t, d, `CREATE EXTENSION IF NOT EXISTS postgres_fdw`)
	mustExec(t, d, `CREATE TABLE inspect_foreign_target_pg_test (id int)`)
	mustExec(t, d, `CREATE SERVER inspect_loopback_pg_test FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host 'localhost', port '5432', dbname 'testdb')`)
	mustExec(t, d, `CREATE USER MAPPING FOR CURRENT_USER SERVER inspect_loopback_pg_test OPTIONS (user 'testuser', password 'testpass')`)
	mustExec(t, d, `CREATE FOREIGN TABLE inspect_foreign_pg_test (id int) SERVER inspect_loopback_pg_test OPTIONS (schema_name 'public', table_name 'inspect_foreign_target_pg_test')`)

	scope := pgTestScope("public")
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "type", Name: "mood"},
		{Scope: scope, Kind: "domain", Name: "positive_int"},
		{Scope: scope, Kind: "table", Name: "widgets"},
		{Scope: scope, Kind: "procedure", Name: "noop"},
		{Scope: scope, Kind: "trigger", Name: "widgets_trg"},
		{Scope: scope, Kind: "table", Name: "events_p"},
		{Scope: scope, Kind: "foreign_table", Name: "inspect_foreign_pg_test"},
	}

	objs, err := d.InspectObjects(ctx, refs)
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	inspected := map[string]bool{}
	for _, obj := range objs {
		inspected[obj.Ref.Kind+"/"+obj.Ref.Name] = true
	}
	for _, ref := range refs {
		if !inspected[ref.Kind+"/"+ref.Name] {
			t.Errorf("InspectObjects returned no object for %s %q", ref.Kind, ref.Name)
		}
	}
	var foundPartitions bool
	for _, obj := range objs {
		if obj.Ref.Name != "events_p" {
			continue
		}
		for _, desc := range obj.Descriptors {
			if desc.Title == "Partitions" && desc.Rows != nil &&
				len(desc.Rows.Rows) == 1 && desc.Rows.Rows[0][0] == "events_p_2026" {
				foundPartitions = true
			}
		}
	}
	if !foundPartitions {
		t.Error("expected events_p to carry a Partitions descriptor listing events_p_2026")
	}
}

// TestPostgresTriggerObjectsFoldsSharedName proves a trigger name reused across
// two tables in one schema yields exactly one object (Postgres trigger names are
// unique per table, but the directory keys objects by schema/kind/name).
func TestPostgresTriggerObjectsFoldsSharedName(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyPostgres(t, d,
			"DROP TABLE IF EXISTS trg_fold_a CASCADE",
			"DROP TABLE IF EXISTS trg_fold_b CASCADE",
			"DROP FUNCTION IF EXISTS trg_fold_fn()",
		)
	})

	mustExec(t, d, `CREATE FUNCTION trg_fold_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TABLE trg_fold_a (id int)`)
	mustExec(t, d, `CREATE TABLE trg_fold_b (id int)`)
	mustExec(t, d, `CREATE TRIGGER set_updated_at BEFORE INSERT ON trg_fold_a FOR EACH ROW EXECUTE FUNCTION trg_fold_fn()`)
	mustExec(t, d, `CREATE TRIGGER set_updated_at BEFORE INSERT ON trg_fold_b FOR EACH ROW EXECUTE FUNCTION trg_fold_fn()`)

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}),
		Kind:  "trigger",
		Name:  "set_updated_at",
	}
	objs, err := TriggerObjects(ctx, d.db, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("TriggerObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("want 1 folded trigger object, got %d", len(objs))
	}
	var tableField string
	for _, desc := range objs[0].Descriptors {
		for _, f := range desc.Fields {
			if f.Name == "Table" {
				tableField = f.Value
			}
		}
	}
	if tableField != "trg_fold_a, trg_fold_b" {
		t.Fatalf("Table field must list both tables, got %q", tableField)
	}
}

// TestPostgresProcedureObjectsFoldsOverloads proves two procedures sharing a
// name in one schema fold into a single object carrying one descriptor per
// overload, matching FunctionObjects and keeping the snapshot PK unique.
func TestPostgresProcedureObjectsFoldsOverloads(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyPostgres(t, d,
			"DROP PROCEDURE IF EXISTS proc_fold(int)",
			"DROP PROCEDURE IF EXISTS proc_fold(text)",
		)
	})

	mustExec(t, d, `CREATE PROCEDURE proc_fold(a int) LANGUAGE plpgsql AS $$ BEGIN END $$`)
	mustExec(t, d, `CREATE PROCEDURE proc_fold(a text) LANGUAGE plpgsql AS $$ BEGIN END $$`)

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}),
		Kind:  "procedure",
		Name:  "proc_fold",
	}
	objs, err := ProcedureObjects(ctx, d.db, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("ProcedureObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("want 1 folded procedure object, got %d", len(objs))
	}
	if len(objs[0].Descriptors) != 2 {
		t.Fatalf("want 2 overload descriptors, got %d", len(objs[0].Descriptors))
	}
}

// dropQuietlyPostgres runs cleanup DROP statements whose targets may already
// be gone, ignoring errors so cleanup ordering does not matter.
func dropQuietlyPostgres(t *testing.T, d *Driver, statements ...string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range statements {
		_, _ = d.Execute(ctx, stmt)
	}
}
