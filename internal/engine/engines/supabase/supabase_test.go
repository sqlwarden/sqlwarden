package supabase

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = connStr

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "supabase"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestRegistered(t *testing.T) {
	d, err := engine.New("supabase")
	if err != nil {
		t.Fatalf("engine.New(supabase): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(supabase) returned %T, want *driver", d)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("supabase")
	if !ok {
		t.Fatal("supabase not registered")
	}
	if caps.Engine.ID != "supabase" || caps.Engine.DisplayName != "Supabase" || caps.Engine.Dialect != engine.DialectPostgres {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	postgresCaps, _ := engine.Describe("postgres")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLClassify, engine.CapabilitySQLComplete, engine.CapabilitySQLSafetyCheck,
		engine.CapabilitySQLExplain, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != postgresCaps.Capabilities[capID] {
			t.Errorf("capability %q: supabase=%v postgres=%v", capID, caps.Capabilities[capID], postgresCaps.Capabilities[capID])
		}
	}
}

func TestDialect(t *testing.T) {
	d := &driver{}
	if got := d.Dialect(); got != engine.DialectPostgres {
		t.Errorf("Dialect() = %q, want %q", got, engine.DialectPostgres)
	}
}

func TestConnect(t *testing.T) {
	d := connect(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestInspectDirectoryExcludesManagedSchemas(t *testing.T) {
	d := connect(t)
	ctx := context.Background()

	for schema := range managedSchemas {
		if _, err := d.Execute(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			t.Fatalf("create managed schema %q: %v", schema, err)
		}
	}
	if _, err := d.Execute(ctx, `CREATE TABLE IF NOT EXISTS auth.users (id int)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Execute(ctx, `CREATE SCHEMA IF NOT EXISTS app`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE IF NOT EXISTS app.widgets (id int)`); err != nil {
		t.Fatal(err)
	}

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, node := range dir.ScopeNodes() {
		if managed := node.Path.Name("schema"); managedSchemas[managed] {
			t.Fatalf("directory included managed schema %q", managed)
		}
	}

	foundWidgets := false
	for _, ref := range dir.ObjectRefs() {
		if ref.Kind == "table" && ref.Name == "widgets" && ref.Scope.Name("schema") == "app" {
			foundWidgets = true
		}
	}
	if !foundWidgets {
		t.Fatal("expected app.widgets to be present in the directory")
	}
}

func TestDiscoverScopesExcludesManagedSchemas(t *testing.T) {
	d := connect(t)
	ctx := context.Background()

	for schema := range managedSchemas {
		if _, err := d.Execute(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			t.Fatalf("create managed schema %q: %v", schema, err)
		}
	}
	if _, err := d.Execute(ctx, `CREATE SCHEMA IF NOT EXISTS app`); err != nil {
		t.Fatal(err)
	}

	discovery, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{
		Parent: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	foundApp := false
	for _, scope := range discovery.Scopes {
		name := scope.Name("schema")
		if managedSchemas[name] {
			t.Fatalf("discovered managed schema %q", name)
		}
		if name == "app" {
			foundApp = true
		}
	}
	if !foundApp {
		t.Fatal("expected app schema to be discoverable")
	}
}

// TestInspectDirectoryPopulatesEveryAdvertisedKind proves the hand-rolled
// InspectDirectory catalogues every object kind the inherited postgres
// SchemaSpec advertises. The override composes the catalog functions by hand
// rather than delegating, so a kind added to the base spec is otherwise
// advertised to Supabase users and then left permanently empty.
func TestInspectDirectoryPopulatesEveryAdvertisedKind(t *testing.T) {
	d := connect(t)
	ctx := context.Background()

	for _, statement := range []string{
		`CREATE SCHEMA IF NOT EXISTS kinds`,
		`CREATE TABLE IF NOT EXISTS kinds.gadgets (id int PRIMARY KEY, name text)`,
		`CREATE VIEW kinds.gadget_names AS SELECT name FROM kinds.gadgets`,
		`CREATE MATERIALIZED VIEW IF NOT EXISTS kinds.gadget_count AS SELECT count(*) FROM kinds.gadgets`,
		`CREATE SEQUENCE IF NOT EXISTS kinds.gadget_seq`,
		`CREATE TYPE kinds.gadget_state AS ENUM ('new', 'used')`,
		`CREATE DOMAIN kinds.positive_int AS int CHECK (VALUE > 0)`,
		`CREATE FUNCTION kinds.gadget_touch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`,
		`CREATE TRIGGER gadget_touched BEFORE UPDATE ON kinds.gadgets FOR EACH ROW EXECUTE FUNCTION kinds.gadget_touch()`,
		`CREATE PROCEDURE kinds.gadget_noop() LANGUAGE plpgsql AS $$ BEGIN END $$`,
		`CREATE EXTENSION IF NOT EXISTS postgres_fdw`,
		`CREATE SERVER IF NOT EXISTS kinds_server FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host 'localhost', dbname 'testdb')`,
		`CREATE FOREIGN TABLE IF NOT EXISTS kinds.remote_gadgets (id int) SERVER kinds_server`,
	} {
		if _, err := d.Execute(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), `DROP SCHEMA IF EXISTS kinds CASCADE`)
		_, _ = d.Execute(context.Background(), `DROP SERVER IF EXISTS kinds_server CASCADE`)
	})

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, ref := range dir.ObjectRefs() {
		if ref.Scope.Name("schema") == "kinds" {
			seen[ref.Kind] = true
		}
	}
	for _, kind := range []string{
		"table", "view", "materialized_view", "function", "sequence",
		"procedure", "trigger", "type", "domain", "foreign_table",
	} {
		if !seen[kind] {
			t.Errorf("kind %q is advertised by SchemaSpec but was not populated", kind)
		}
	}
}
