package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

// TestPostgresInspectObjectsCoversNewKinds proves every new schema-object kind
// added for Postgres/MySQL parity (type, domain, procedure, trigger,
// partitioned tables, and foreign tables) shows up in both directory listing
// and bulk object inspection against a live database.
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

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	refs := dir.ObjectRefs()
	byKind := map[string]bool{}
	for _, ref := range refs {
		byKind[ref.Kind] = true
	}
	for _, kind := range []string{"type", "domain", "procedure", "trigger", "table", "foreign_table"} {
		if !byKind[kind] {
			t.Errorf("expected directory to contain a %q ref", kind)
		}
	}

	objs, err := d.InspectObjects(ctx, refs)
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
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

// dropQuietlyPostgres runs cleanup DROP statements whose targets may already
// be gone, ignoring errors so cleanup ordering does not matter.
func dropQuietlyPostgres(t *testing.T, d *Driver, statements ...string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range statements {
		_, _ = d.Execute(ctx, stmt)
	}
}
