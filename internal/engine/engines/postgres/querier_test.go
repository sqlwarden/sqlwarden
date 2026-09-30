package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

func databaseScope(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
}

func TestDefaultScopeDatabaseOverridesDSN(t *testing.T) {
	setup := newConnectedDriver(t)
	mustExec(t, setup, `CREATE DATABASE navigator_default`)
	t.Cleanup(func() { mustExec(t, setup, `DROP DATABASE IF EXISTS navigator_default WITH (FORCE)`) })

	d := &Driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "postgres", DefaultScope: databaseScope("navigator_default")}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	var current string
	if err := d.db.QueryRowContext(context.Background(), `SELECT current_database()`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != "navigator_default" {
		t.Fatalf("current_database = %q", current)
	}
}

func TestQuerierRoutesToOtherDatabase(t *testing.T) {
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE DATABASE navigator_other`)
	t.Cleanup(func() { mustExec(t, d, `DROP DATABASE IF EXISTS navigator_other WITH (FORCE)`) })

	ctx := context.Background()
	primary, err := d.Querier(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if primary != d.db {
		t.Fatal("empty database must return the primary pool")
	}
	q, err := d.Querier(ctx, "navigator_other")
	if err != nil {
		t.Fatalf("Querier: %v", err)
	}
	var current string
	if err := q.QueryRowContext(ctx, `SELECT current_database()`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != "navigator_other" {
		t.Fatalf("current_database = %q", current)
	}
	again, _ := d.Querier(ctx, "navigator_other")
	if again != q {
		t.Fatal("clone pool must be reused per database")
	}
}

func TestQuerierFailsForDatabaseWithoutConnectPrivilege(t *testing.T) {
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE ROLE navigator_limited LOGIN PASSWORD 'limited'`)
	mustExec(t, d, `CREATE DATABASE navigator_locked`)
	mustExec(t, d, `REVOKE CONNECT ON DATABASE navigator_locked FROM PUBLIC`)
	t.Cleanup(func() {
		mustExec(t, d, `DROP DATABASE IF EXISTS navigator_locked WITH (FORCE)`)
		mustExec(t, d, `DROP ROLE IF EXISTS navigator_limited`)
	})
	limitedDSN := replaceDSNUser(t, testDSN, "navigator_limited", "limited")
	limited := &Driver{}
	if err := limited.Connect(context.Background(), engine.ConnectionConfig{DSN: limitedDSN, Driver: "postgres"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = limited.Close() })
	if _, err := limited.Querier(context.Background(), "navigator_locked"); err == nil {
		t.Fatal("expected an error for a database without CONNECT privilege")
	}
	if _, err := limited.Querier(context.Background(), "navigator_locked"); err == nil {
		t.Fatal("a failed clone must not be cached")
	}
}

func TestInspectObjectsQualifiesDatabase(t *testing.T) {
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE TABLE nav_parent (id int primary key)`)
	mustExec(t, d, `CREATE TABLE nav_child (id int primary key, parent_id int references nav_parent(id))`)
	t.Cleanup(func() { mustExec(t, d, `DROP TABLE IF EXISTS nav_child, nav_parent`) })

	var dbName string
	_ = d.db.QueryRowContext(context.Background(), `SELECT current_database()`).Scan(&dbName)
	scope := databaseScope(dbName).Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	objs, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{{Scope: scope, Kind: "table", Name: "nav_child"}})
	if err != nil || len(objs) != 1 {
		t.Fatalf("InspectObjects = %v, %v", objs, err)
	}
	if objs[0].Ref.Scope != scope {
		t.Fatalf("ref scope = %q", objs[0].Ref.Scope)
	}
	fk := objs[0].Relational.ForeignKeys[0]
	if fk.References.Scope != scope {
		t.Fatalf("fk target scope = %q, want %q", fk.References.Scope, scope)
	}
}
