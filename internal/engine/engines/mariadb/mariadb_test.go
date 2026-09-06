package mariadb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"

	tcmariadb "github.com/testcontainers/testcontainers-go/modules/mariadb"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcmariadb.Run(ctx,
		"mariadb:11.4",
		tcmariadb.WithDatabase("testdb"),
		tcmariadb.WithUsername("testuser"),
		tcmariadb.WithPassword("testpass"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start mariadb container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = connStr

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "mariadb"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRegistered(t *testing.T) {
	d, err := engine.New("mariadb")
	if err != nil {
		t.Fatalf("engine.New(mariadb): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(mariadb) returned %T, want *driver", d)
	}
}

func TestDialect(t *testing.T) {
	d := &driver{}
	if got := d.Dialect(); got != engine.DialectMariaDB {
		t.Errorf("Dialect() = %q, want %q", got, engine.DialectMariaDB)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("mariadb")
	if !ok {
		t.Fatal("mariadb not registered")
	}
	if caps.Engine.ID != "mariadb" || caps.Engine.DisplayName != "MariaDB" || caps.Engine.Dialect != engine.DialectMariaDB {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	mysqlCaps, _ := engine.Describe("mysql")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLParse, engine.CapabilitySQLClassify, engine.CapabilitySQLComplete,
		engine.CapabilitySQLSafetyCheck, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != mysqlCaps.Capabilities[capID] {
			t.Errorf("capability %q: mariadb=%v mysql=%v", capID, caps.Capabilities[capID], mysqlCaps.Capabilities[capID])
		}
	}
}

func TestConnect(t *testing.T) {
	d := connect(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSchemaSpecIncludesSequence(t *testing.T) {
	d := &driver{}
	spec := d.SchemaSpec()
	if spec.Dialect != "mariadb" {
		t.Fatalf("Dialect = %q, want mariadb", spec.Dialect)
	}
	var found bool
	for _, kind := range spec.Kinds {
		if kind.Kind == "sequence" {
			found = true
		}
	}
	if !found {
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

func TestInspectObjectsReportsJSONColumnType(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP TABLE IF EXISTS json_col_test")
	t.Cleanup(func() { exec("DROP TABLE IF EXISTS json_col_test") })
	exec("CREATE TABLE json_col_test (id INT PRIMARY KEY, payload JSON, label VARCHAR(20))")

	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{{Scope: scope, Kind: "table", Name: "json_col_test"}})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected one object, got %d", len(objects))
	}
	var payload, label *metadata.Column
	for i := range objects[0].Relational.Columns {
		col := &objects[0].Relational.Columns[i]
		switch col.Name {
		case "payload":
			payload = col
		case "label":
			label = col
		}
	}
	if payload == nil || !strings.EqualFold(payload.DataType, "json") {
		t.Fatalf("payload column data type = %+v, want json", payload)
	}
	if label == nil || strings.EqualFold(label.DataType, "json") {
		t.Fatalf("label column should not be reported as json: %+v", label)
	}
}

func TestConnectionContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := &driver{}
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "mariadb"}); err != nil {
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
