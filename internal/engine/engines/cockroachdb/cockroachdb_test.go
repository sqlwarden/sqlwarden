package cockroachdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"

	tccockroachdb "github.com/testcontainers/testcontainers-go/modules/cockroachdb"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tccockroachdb.Run(ctx,
		"cockroachdb/cockroach:v23.1.13",
		tccockroachdb.WithInsecure(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start cockroachdb container: %v\n", err)
		os.Exit(1)
	}

	// ConnectionString returns a stdlib-registered pseudo-DSN that pgx.ParseConfig
	// rejects; ConnectionConfig's own ConnString is the parseable form this
	// driver's pgx.ParseConfig-based Connect needs.
	cfg, err := container.ConnectionConfig(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection config: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	testDSN = cfg.ConnString()

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "cockroachdb"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRegistered(t *testing.T) {
	d, err := engine.New("cockroachdb")
	if err != nil {
		t.Fatalf("engine.New(cockroachdb): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(cockroachdb) returned %T, want *driver", d)
	}
}

func TestDialect(t *testing.T) {
	d := &driver{}
	if got := d.Dialect(); got != engine.DialectCockroachDB {
		t.Errorf("Dialect() = %q, want %q", got, engine.DialectCockroachDB)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("cockroachdb")
	if !ok {
		t.Fatal("cockroachdb not registered")
	}
	if caps.Engine.ID != "cockroachdb" || caps.Engine.DisplayName != "CockroachDB" || caps.Engine.Dialect != engine.DialectCockroachDB {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	postgresCaps, _ := engine.Describe("postgres")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLParse, engine.CapabilitySQLClassify, engine.CapabilitySQLComplete,
		engine.CapabilitySQLSafetyCheck, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != postgresCaps.Capabilities[capID] {
			t.Errorf("capability %q: cockroachdb=%v postgres=%v", capID, caps.Capabilities[capID], postgresCaps.Capabilities[capID])
		}
	}
}

func TestConnect(t *testing.T) {
	d := connect(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSchemaSpecOmitsMaterializedView(t *testing.T) {
	d := &driver{}
	spec := d.SchemaSpec()
	if spec.Dialect != "cockroachdb" {
		t.Fatalf("Dialect = %q, want cockroachdb", spec.Dialect)
	}
	for _, kind := range spec.Kinds {
		if kind.Kind == "materialized_view" {
			t.Fatalf("materialized_view must not be reported for cockroachdb: %+v", spec.Kinds)
		}
	}
}

func TestInspectDirectoryOmitsMaterializedView(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP TABLE IF EXISTS crdb_directory_test")
	t.Cleanup(func() { exec("DROP TABLE IF EXISTS crdb_directory_test") })
	exec("CREATE TABLE crdb_directory_test (id INT PRIMARY KEY)")

	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	for _, ref := range directory.ObjectRefs() {
		if ref.Kind == "materialized_view" {
			t.Fatalf("directory must not contain a materialized_view ref: %+v", ref)
		}
	}
	var found bool
	for _, ref := range directory.ObjectRefs() {
		if ref.Kind == "table" && ref.Name == "crdb_directory_test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("directory missing crdb_directory_test: %+v", directory.Roots)
	}
}

func TestInspectDirectoryExcludesSystemSchemas(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	for _, ref := range directory.ObjectRefs() {
		schema := ref.Scope.Name("schema")
		if schema == "crdb_internal" || schema == "pg_extension" {
			t.Fatalf("directory must not contain refs from system schema %q: %+v", schema, ref)
		}
	}
}

func TestDiscoverScopesExcludesSystemSchemas(t *testing.T) {
	d := connect(t)
	ctx := context.Background()

	discovery, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{
		Parent: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "defaultdb"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	foundPublic := false
	for _, scope := range discovery.Scopes {
		name := scope.Name("schema")
		if name == "crdb_internal" || name == "pg_extension" {
			t.Fatalf("discovered system schema %q", name)
		}
		if name == "public" {
			foundPublic = true
		}
	}
	if !foundPublic {
		t.Fatal("expected public schema to be discoverable")
	}
}

func TestInspectObjectsForFunction(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "defaultdb"}).
		Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP FUNCTION IF EXISTS crdb_fn_test")
	t.Cleanup(func() { exec("DROP FUNCTION IF EXISTS crdb_fn_test") })
	exec("CREATE FUNCTION crdb_fn_test(a INT, b INT) RETURNS INT AS $$ SELECT a + b $$ LANGUAGE SQL")

	ref := metadata.ObjectRef{Scope: scope, Kind: "function", Name: "crdb_fn_test"}
	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected one function object, got %+v", objects)
	}

	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Kind != "source" || desc.Source.Language != "sql" {
		t.Fatalf("function definition descriptor = %+v", desc)
	}
	if !strings.Contains(strings.ToUpper(desc.Source.Body), "LANGUAGE SQL") {
		t.Fatalf("function definition body missing LANGUAGE SQL: %q", desc.Source.Body)
	}
}

func TestInspectObjectsAndDefinitionForTable(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "defaultdb"}).
		Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP TABLE IF EXISTS crdb_object_test")
	t.Cleanup(func() { exec("DROP TABLE IF EXISTS crdb_object_test") })
	exec("CREATE TABLE crdb_object_test (id INT PRIMARY KEY, label TEXT NOT NULL)")

	ref := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "crdb_object_test"}
	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 || objects[0].Relational == nil {
		t.Fatalf("expected one table object, got %+v", objects)
	}
	if got := len(objects[0].Relational.Columns); got != 2 {
		t.Fatalf("expected two columns, got %d: %+v", got, objects[0].Relational.Columns)
	}

	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Kind != "source" || !strings.Contains(strings.ToUpper(desc.Source.Body), "CREATE TABLE") {
		t.Fatalf("table definition descriptor = %+v", desc)
	}
}

func TestInspectDirectoryReportsSequence(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "defaultdb"}).
		Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP SEQUENCE IF EXISTS crdb_seq_test")
	t.Cleanup(func() { exec("DROP SEQUENCE IF EXISTS crdb_seq_test") })
	exec("CREATE SEQUENCE crdb_seq_test START 1 INCREMENT 1")

	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	if !directoryHasRef(directory, metadata.ObjectRef{Scope: scope, Kind: "sequence", Name: "crdb_seq_test"}) {
		t.Fatalf("directory missing sequence: %+v", directory.Roots)
	}
}

func TestExplainUsesCockroachDBGrammar(t *testing.T) {
	d := &driver{}
	plain, err := d.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatalf("Explain plain: %v", err)
	}
	if plain.Statement != "EXPLAIN SELECT 1" {
		t.Fatalf("plain statement = %q, want %q", plain.Statement, "EXPLAIN SELECT 1")
	}
	analyze, err := d.Explain("SELECT 1", explain.ModeAnalyze)
	if err != nil {
		t.Fatalf("Explain analyze: %v", err)
	}
	if analyze.Statement != "EXPLAIN ANALYZE SELECT 1" {
		t.Fatalf("analyze statement = %q, want %q", analyze.Statement, "EXPLAIN ANALYZE SELECT 1")
	}
}

func TestExplainRejectsMultipleStatements(t *testing.T) {
	d := &driver{}
	if _, err := d.Explain("SELECT 1; SELECT 2", explain.ModePlain); err != explain.ErrMultipleStatements {
		t.Fatalf("Explain multi-statement error = %v, want %v", err, explain.ErrMultipleStatements)
	}
}

func TestExplainRejectsAlreadyExplained(t *testing.T) {
	d := &driver{}
	if _, err := d.Explain("EXPLAIN SELECT 1", explain.ModePlain); err != explain.ErrAlreadyExplained {
		t.Fatalf("Explain already-explained error = %v, want %v", err, explain.ErrAlreadyExplained)
	}
}

func TestConnectionContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := &driver{}
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "cockroachdb"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if _, err := d.Query(ctx, "SELECT 1"); err != nil {
		t.Fatalf("Query: %v", err)
	}
}

func TestMalformedDSNRejected(t *testing.T) {
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: "not a dsn", Driver: "cockroachdb"}); err == nil {
		t.Fatal("expected Connect to reject a malformed DSN")
	}
}

func directoryHasRef(directory *metadata.Directory, ref metadata.ObjectRef) bool {
	for _, got := range directory.ObjectRefs() {
		if got == ref {
			return true
		}
	}
	return false
}
