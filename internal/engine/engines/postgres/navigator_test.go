package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func currentDatabase(t *testing.T, d *Driver) string {
	t.Helper()
	var name string
	if err := d.db.QueryRowContext(context.Background(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func schemaPath(database, schema string) metadata.ScopePath {
	return databaseScope(database).Child(metadata.ScopeSegment{Kind: "schema", Name: schema})
}

func relationPath(database, schema, kind, name string) metadata.ScopePath {
	return schemaPath(database, schema).Child(metadata.ScopeSegment{Kind: kind, Name: name})
}

func childNames(children []metadata.Child) []string {
	names := make([]string, 0, len(children))
	for _, c := range children {
		names = append(names, c.Kind+":"+c.Name)
	}
	return names
}

func findChild(t *testing.T, children []metadata.Child, name string) metadata.Child {
	t.Helper()
	for _, c := range children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("child %q not found in %v", name, childNames(children))
	return metadata.Child{}
}

func seedRelationFixture(t *testing.T, d *Driver) string {
	t.Helper()
	mustExec(t, d, `CREATE SCHEMA nav_rel`)
	mustExec(t, d, `CREATE TABLE nav_rel.users (id int PRIMARY KEY, email text UNIQUE NOT NULL, age int CHECK (age > 0))`)
	mustExec(t, d, `CREATE TABLE nav_rel.orders (id int PRIMARY KEY, user_id int CONSTRAINT fk_owner REFERENCES nav_rel.users(id))`)
	mustExec(t, d, `CREATE TABLE nav_rel.invoices (id int PRIMARY KEY, user_id int CONSTRAINT fk_owner REFERENCES nav_rel.users(id))`)
	mustExec(t, d, `CREATE INDEX users_age_idx ON nav_rel.users (age)`)
	mustExec(t, d, `CREATE VIEW nav_rel.active_users AS SELECT id FROM nav_rel.users`)
	mustExec(t, d, `CREATE FUNCTION nav_rel.touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, d, `CREATE TRIGGER users_touch BEFORE UPDATE ON nav_rel.users FOR EACH ROW EXECUTE FUNCTION nav_rel.touch()`)
	mustExec(t, d, `CREATE RULE users_noop AS ON DELETE TO nav_rel.users DO INSTEAD NOTHING`)
	mustExec(t, d, `ALTER TABLE nav_rel.users ENABLE ROW LEVEL SECURITY`)
	mustExec(t, d, `CREATE POLICY users_self ON nav_rel.users FOR SELECT USING (true)`)
	mustExec(t, d, `CREATE TABLE nav_rel.events (id int, at date) PARTITION BY RANGE (at)`)
	mustExec(t, d, `CREATE TABLE nav_rel.events_2026 PARTITION OF nav_rel.events FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS nav_rel CASCADE`) })
	return currentDatabase(t, d)
}

func TestRelationLevelLoaders(t *testing.T) {
	d := newConnectedDriver(t)
	db := seedRelationFixture(t, d)
	ctx := context.Background()
	users := relationPath(db, "nav_rel", "table", "users")
	orders := relationPath(db, "nav_rel", "table", "orders")
	events := relationPath(db, "nav_rel", "table", "events")
	both := []metadata.ScopePath{users, orders}

	cols, err := ListColumns(ctx, d.db, both)
	if err != nil {
		t.Fatal(err)
	}
	id := findChild(t, cols[users], "id")
	if id.Attributes["primary_key"] != true || id.Attributes["data_type"] != "integer" || id.Attributes["nullable"] != false {
		t.Fatalf("users.id attributes = %v", id.Attributes)
	}
	if findChild(t, cols[orders], "user_id").Attributes["foreign_key"] != true {
		t.Fatal("orders.user_id must be flagged foreign_key")
	}
	if got := childNames(cols[users]); got[0] != "column:id" || got[1] != "column:email" || got[2] != "column:age" {
		t.Fatalf("columns must be in ordinal order: %v", got)
	}

	cons := mustListing(t)(ListConstraints(ctx, d.db, []metadata.ScopePath{users}))
	types := map[string]bool{}
	for _, c := range cons[users] {
		types[c.Attributes["constraint_type"].(string)] = true
	}
	if !types["primary_key"] || !types["unique"] || !types["check"] {
		t.Fatalf("constraint types = %v", types)
	}

	fks := mustListing(t)(ListForeignKeys(ctx, d.db, []metadata.ScopePath{orders}))
	if findChild(t, fks[orders], "fk_owner").Attributes["referenced_table"] != "nav_rel.users" {
		t.Fatalf("fk = %v", fks[orders])
	}

	refs := mustListing(t)(ListReferences(ctx, d.db, []metadata.ScopePath{users}))
	if len(refs[users]) != 2 {
		t.Fatalf("users must be referenced twice (duplicate names allowed): %v", refs[users])
	}

	idx := mustListing(t)(ListRelationIndexes(ctx, d.db, []metadata.ScopePath{users}))
	if findChild(t, idx[users], "users_age_idx").Attributes["method"] != "btree" {
		t.Fatal("index method missing")
	}
	schemaIdx := mustListing(t)(ListSchemaIndexes(ctx, d.db, []metadata.ScopePath{schemaPath(db, "nav_rel")}))
	if findChild(t, schemaIdx[schemaPath(db, "nav_rel")], "users_age_idx").Attributes["table"] != "users" {
		t.Fatal("schema-level index must name its table")
	}

	deps := mustListing(t)(ListDependencies(ctx, d.db, []metadata.ScopePath{users}))
	if findChild(t, deps[users], "nav_rel.active_users").Attributes["object_kind"] != "view" {
		t.Fatalf("dependencies = %v", deps[users])
	}

	parts := mustListing(t)(ListPartitions(ctx, d.db, []metadata.ScopePath{events}))
	if findChild(t, parts[events], "events_2026").Attributes["bound"] == "" {
		t.Fatal("partition bound missing")
	}

	trig := mustListing(t)(ListTriggers(ctx, d.db, []metadata.ScopePath{users}))
	if got := childNames(trig[users]); len(got) != 1 || got[0] != "trigger:users_touch" {
		t.Fatalf("triggers must exclude internal FK triggers: %v", got)
	}
	rules := mustListing(t)(ListRules(ctx, d.db, []metadata.ScopePath{users}))
	if got := childNames(rules[users]); len(got) != 1 || got[0] != "rule:users_noop" {
		t.Fatalf("rules = %v", got)
	}
	viewRules := mustListing(t)(ListRules(ctx, d.db, []metadata.ScopePath{relationPath(db, "nav_rel", "view", "active_users")}))
	for _, r := range viewRules[relationPath(db, "nav_rel", "view", "active_users")] {
		if r.Name == "_RETURN" {
			t.Fatal("_RETURN rule must be hidden")
		}
	}
	pols := mustListing(t)(ListPolicies(ctx, d.db, []metadata.ScopePath{users}))
	if findChild(t, pols[users], "users_self").Attributes["command"] != "select" {
		t.Fatalf("policies = %v", pols[users])
	}
}

func TestListDatabasesMarksCurrent(t *testing.T) {
	d := newConnectedDriver(t)
	out, err := ListDatabases(context.Background(), d.db, []metadata.ScopePath{""})
	if err != nil {
		t.Fatal(err)
	}
	current := findChild(t, out[""], currentDatabase(t, d))
	if !current.Current || current.Kind != "database" {
		t.Fatalf("current database = %+v", current)
	}
	for _, c := range out[""] {
		if c.Name == "template0" || c.Name == "template1" {
			t.Fatalf("templates must be excluded: %v", childNames(out[""]))
		}
	}
}

func TestListSchemasFlagsSystemAndCurrent(t *testing.T) {
	d := newConnectedDriver(t)
	parent := databaseScope(currentDatabase(t, d))
	out, err := ListSchemas(context.Background(), d.db, []metadata.ScopePath{parent})
	if err != nil {
		t.Fatal(err)
	}
	if !findChild(t, out[parent], "pg_catalog").System || !findChild(t, out[parent], "information_schema").System {
		t.Fatal("catalog schemas must be system")
	}
	public := findChild(t, out[parent], "public")
	if public.System || !public.Current {
		t.Fatalf("public = %+v", public)
	}
}

func TestListRelationsBatchedAcrossSchemas(t *testing.T) {
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE SCHEMA nav_a`)
	mustExec(t, d, `CREATE SCHEMA nav_b`)
	mustExec(t, d, `CREATE TABLE nav_a.t1 (id int)`)
	mustExec(t, d, `CREATE TABLE nav_b.t2 (id int, created date) PARTITION BY RANGE (created)`)
	mustExec(t, d, `CREATE TABLE nav_b.t2_2026 PARTITION OF nav_b.t2 FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	mustExec(t, d, `CREATE VIEW nav_a.v1 AS SELECT 1 AS x`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW nav_a.mv1 AS SELECT 1 AS x`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS nav_a, nav_b CASCADE`) })

	db := currentDatabase(t, d)
	a, b := schemaPath(db, "nav_a"), schemaPath(db, "nav_b")
	ctx := context.Background()

	tables, err := ListTables(ctx, d.db, []metadata.ScopePath{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(tables[a]); len(got) != 1 || got[0] != "table:t1" {
		t.Fatalf("nav_a tables = %v", got)
	}
	if got := childNames(tables[b]); len(got) != 1 || got[0] != "table:t2" {
		t.Fatalf("nav_b tables must exclude partitions: %v", got)
	}
	if findChild(t, tables[b], "t2").Attributes["partitioned"] != true {
		t.Fatal("partitioned attribute missing")
	}
	views, err := ListViews(ctx, d.db, []metadata.ScopePath{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(views[a]); len(got) != 1 || got[0] != "view:v1" {
		t.Fatalf("views = %v", got)
	}
	mviews, err := ListMaterializedViews(ctx, d.db, []metadata.ScopePath{a})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(mviews[a]); len(got) != 1 || got[0] != "materialized_view:mv1" {
		t.Fatalf("materialized views = %v", got)
	}
}

func mustListing(t *testing.T) func(map[metadata.ScopePath][]metadata.Child, error) map[metadata.ScopePath][]metadata.Child {
	return func(listing map[metadata.ScopePath][]metadata.Child, err error) map[metadata.ScopePath][]metadata.Child {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return listing
	}
}

func TestSchemaAndDatabaseLevelLoaders(t *testing.T) {
	d := newConnectedDriver(t)
	mustExec(t, d, `CREATE SCHEMA nav_s`)
	mustExec(t, d, `CREATE FUNCTION nav_s.add(a int, b int) RETURNS int LANGUAGE sql AS 'SELECT a + b'`)
	mustExec(t, d, `CREATE FUNCTION nav_s.add(a text, b text) RETURNS text LANGUAGE sql AS 'SELECT a || b'`)
	mustExec(t, d, `CREATE PROCEDURE nav_s.run() LANGUAGE sql AS 'SELECT 1'`)
	mustExec(t, d, `CREATE AGGREGATE nav_s.total(int) (SFUNC = int4pl, STYPE = int)`)
	mustExec(t, d, `CREATE SEQUENCE nav_s.counter`)
	mustExec(t, d, `CREATE TYPE nav_s.mood AS ENUM ('ok')`)
	mustExec(t, d, `CREATE DOMAIN nav_s.positive AS int CHECK (VALUE > 0)`)
	mustExec(t, d, `CREATE TABLE nav_s.plain (id int)`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS nav_s CASCADE`) })

	db := currentDatabase(t, d)
	s := schemaPath(db, "nav_s")
	ctx := context.Background()

	routines, err := ListRoutines(ctx, d.db, []metadata.ScopePath{s})
	if err != nil {
		t.Fatal(err)
	}
	if got := childNames(routines[s]); len(got) != 2 || got[0] != "function:add" || got[1] != "procedure:run" {
		t.Fatalf("routines must dedupe overloads and include procedures: %v", got)
	}
	if findChild(t, routines[s], "add").Attributes["overloads"] != int64(2) {
		t.Fatal("overload count missing")
	}
	aggs := mustListing(t)(ListAggregates(ctx, d.db, []metadata.ScopePath{s}))
	if got := childNames(aggs[s]); len(got) != 1 || got[0] != "aggregate:total" {
		t.Fatalf("aggregates = %v", got)
	}
	seqs := mustListing(t)(ListSequences(ctx, d.db, []metadata.ScopePath{s}))
	if got := childNames(seqs[s]); len(got) != 1 || got[0] != "sequence:counter" {
		t.Fatalf("sequences = %v", got)
	}
	types := mustListing(t)(ListDataTypes(ctx, d.db, []metadata.ScopePath{s}))
	if got := childNames(types[s]); len(got) != 2 || got[0] != "type:mood" || got[1] != "domain:positive" {
		t.Fatalf("data types must list enum and domain only (no table row types or arrays): %v", got)
	}

	dbPath := databaseScope(db)
	exts := mustListing(t)(ListExtensions(ctx, d.db, []metadata.ScopePath{dbPath}))
	if findChild(t, exts[dbPath], "plpgsql").Kind != "extension" {
		t.Fatal("plpgsql extension missing")
	}
	if _, err := ListEventTriggers(ctx, d.db, []metadata.ScopePath{dbPath}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresNavigatorContract(t *testing.T) {
	d := newConnectedDriver(t)
	db := seedRelationFixture(t, d)
	mustExec(t, d, `CREATE SCHEMA nav_other`)
	mustExec(t, d, `CREATE TABLE nav_other.solo (id int PRIMARY KEY)`)
	t.Cleanup(func() { mustExec(t, d, `DROP SCHEMA IF EXISTS nav_other CASCADE`) })

	schemas := []metadata.ScopePath{schemaPath(db, "nav_rel"), schemaPath(db, "nav_other")}
	tables := []metadata.ScopePath{relationPath(db, "nav_rel", "table", "users"), relationPath(db, "nav_other", "table", "solo")}
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
		{NodeKind: "database", Folder: "schemas", Parents: []metadata.ScopePath{databaseScope(db)}},
		{NodeKind: "database", Folder: "extensions", Parents: []metadata.ScopePath{databaseScope(db)}},
	}
	for _, folder := range []string{"tables", "views", "materialized_views", "foreign_tables", "indexes", "functions", "sequences", "data_types", "aggregate_functions"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "schema", Folder: folder, Parents: schemas})
	}
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "indexes", "dependencies", "references", "partitions", "triggers", "rules", "policies"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "", cases)
}
