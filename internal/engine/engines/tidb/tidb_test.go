package tidb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

// testDSN targets the "testdb" database created in TestMain. TiDB has no
// dedicated testcontainers module, so the container is started and readied
// by hand: the official pingcap/tidb image runs tidb-server with its
// embedded unistore backend by default, so a single container is a complete,
// PD/TiKV-free standalone cluster suitable for tests.
var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "pingcap/tidb:v7.5.1",
			ExposedPorts: []string{"4000/tcp"},
			WaitingFor: wait.ForSQL("4000/tcp", "mysql", func(host string, port nat.Port) string {
				return fmt.Sprintf("root@tcp(%s:%s)/", host, port.Port())
			}).WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start tidb container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "container host: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	mapped, err := container.MappedPort(ctx, "4000/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "container port: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	rootDSN := fmt.Sprintf("root@tcp(%s:%s)/", host, mapped.Port())
	db, err := sql.Open("mysql", rootDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open root connection: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	if _, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS testdb"); err != nil {
		fmt.Fprintf(os.Stderr, "create testdb: %v\n", err)
		db.Close()
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	db.Close()

	testDSN = fmt.Sprintf("root@tcp(%s:%s)/testdb", host, mapped.Port())

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "tidb"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRegistered(t *testing.T) {
	d, err := engine.New("tidb")
	if err != nil {
		t.Fatalf("engine.New(tidb): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(tidb) returned %T, want *driver", d)
	}
}

func TestDialect(t *testing.T) {
	d := &driver{}
	if got := d.Dialect(); got != engine.DialectMySQL {
		t.Errorf("Dialect() = %q, want %q", got, engine.DialectMySQL)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("tidb")
	if !ok {
		t.Fatal("tidb not registered")
	}
	if caps.Engine.ID != "tidb" || caps.Engine.DisplayName != "TiDB" || caps.Engine.Dialect != engine.DialectMySQL {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	mysqlCaps, _ := engine.Describe("mysql")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLParse, engine.CapabilitySQLClassify, engine.CapabilitySQLComplete,
		engine.CapabilitySQLSafetyCheck, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != mysqlCaps.Capabilities[capID] {
			t.Errorf("capability %q: tidb=%v mysql=%v", capID, caps.Capabilities[capID], mysqlCaps.Capabilities[capID])
		}
	}
}

func TestConnect(t *testing.T) {
	d := connect(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSchemaSpecOmitsRoutinesAndTriggers(t *testing.T) {
	d := &driver{}
	spec := d.SchemaSpec()
	if spec.Dialect != "tidb" {
		t.Fatalf("Dialect = %q, want tidb", spec.Dialect)
	}
	var hasSequence bool
	for _, kind := range spec.Kinds {
		switch kind.Kind {
		case "function", "procedure", "trigger":
			t.Fatalf("unsupported kind %q present in schema spec: %+v", kind.Kind, spec.Kinds)
		case "sequence":
			hasSequence = true
		}
	}
	if !hasSequence {
		t.Fatalf("expected a sequence kind in schema spec: %+v", spec.Kinds)
	}
}

func TestInspectDirectoryClassifiesSequencesSeparately(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP SEQUENCE IF EXISTS seq_orders")
	exec("DROP TABLE IF EXISTS seq_table_test")
	t.Cleanup(func() {
		exec("DROP SEQUENCE IF EXISTS seq_orders")
		exec("DROP TABLE IF EXISTS seq_table_test")
	})
	exec("CREATE SEQUENCE seq_orders START WITH 1 INCREMENT BY 1")
	exec("CREATE TABLE seq_table_test (id INT PRIMARY KEY)")

	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	if !directoryHasRef(directory, metadata.ObjectRef{Scope: scope, Kind: "sequence", Name: "seq_orders"}) {
		t.Fatalf("directory missing sequence: %+v", directory.Roots)
	}
	if directoryHasRef(directory, metadata.ObjectRef{Scope: scope, Kind: "table", Name: "seq_orders"}) {
		t.Fatal("sequence must not also be reported as a table")
	}
	if !directoryHasRef(directory, metadata.ObjectRef{Scope: scope, Kind: "table", Name: "seq_table_test"}) {
		t.Fatalf("directory missing regular table: %+v", directory.Roots)
	}
}

func TestInspectObjectsAndDefinitionForSequence(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP SEQUENCE IF EXISTS seq_detail_test")
	t.Cleanup(func() { exec("DROP SEQUENCE IF EXISTS seq_detail_test") })
	exec("CREATE SEQUENCE seq_detail_test START WITH 5 INCREMENT BY 2")

	ref := metadata.ObjectRef{Scope: scope, Kind: "sequence", Name: "seq_detail_test"}
	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 || len(objects[0].Descriptors) == 0 {
		t.Fatalf("expected one sequence object with descriptors, got %+v", objects)
	}

	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Kind != "source" || desc.Title != "Definition" {
		t.Fatalf("sequence definition descriptor = %+v", desc)
	}
	if !strings.Contains(strings.ToUpper(desc.Source.Body), "CREATE SEQUENCE") {
		t.Fatalf("sequence definition missing CREATE SEQUENCE:\n%s", desc.Source.Body)
	}
}

func TestInspectObjectsAndDefinitionForTable(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP TABLE IF EXISTS tidb_table_test")
	t.Cleanup(func() { exec("DROP TABLE IF EXISTS tidb_table_test") })
	exec("CREATE TABLE tidb_table_test (id INT PRIMARY KEY, label VARCHAR(32))")

	ref := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "tidb_table_test"}
	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 || objects[0].Relational == nil {
		t.Fatalf("expected one relational object, got %+v", objects)
	}
	if len(objects[0].Relational.PrimaryKey) != 1 || objects[0].Relational.PrimaryKey[0] != "id" {
		t.Errorf("primary key = %v", objects[0].Relational.PrimaryKey)
	}

	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Kind != "source" || desc.Title != "DDL" {
		t.Fatalf("table definition descriptor = %+v", desc)
	}
	if !strings.Contains(strings.ToUpper(desc.Source.Body), "CREATE TABLE") {
		t.Fatalf("table DDL missing CREATE TABLE:\n%s", desc.Source.Body)
	}
}

func TestConnectionContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := &driver{}
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "tidb"}); err != nil {
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

func directoryHasRef(directory *metadata.Directory, ref metadata.ObjectRef) bool {
	for _, got := range directory.ObjectRefs() {
		if got == ref {
			return true
		}
	}
	return false
}
