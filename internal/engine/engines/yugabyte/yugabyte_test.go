package yugabyte

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

// testDSN targets the "yugabyte" database YugabyteDB provisions by default.
// YugabyteDB has no dedicated testcontainers module, so the container is
// started and readied by hand: the official image's yugabyted launcher
// starts a complete single-node cluster (YSQL, YCQL, and the master/tserver
// processes) in one container, exposing YSQL on 5433 with pgx-compatible
// wire protocol.
var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "yugabytedb/yugabyte:2024.2.11.0-b36",
			ExposedPorts: []string{"5433/tcp"},
			Cmd:          []string{"bin/yugabyted", "start", "--background=false"},
			WaitingFor: wait.ForSQL("5433/tcp", "pgx", func(host string, port nat.Port) string {
				return fmt.Sprintf("postgres://yugabyte:yugabyte@%s:%s/yugabyte?sslmode=disable", host, port.Port())
			}).WithStartupTimeout(5 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start yugabyte container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "container host: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	mapped, err := container.MappedPort(ctx, "5433/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "container port: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = fmt.Sprintf("postgres://yugabyte:yugabyte@%s:%s/yugabyte?sslmode=disable", host, mapped.Port())

	code := m.Run()

	_ = container.Terminate(ctx)
	os.Exit(code)
}

func connect(t *testing.T) *driver {
	t.Helper()
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "yugabyte"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRegistered(t *testing.T) {
	d, err := engine.New("yugabyte")
	if err != nil {
		t.Fatalf("engine.New(yugabyte): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(yugabyte) returned %T, want *driver", d)
	}
}

func TestDialect(t *testing.T) {
	d := &driver{}
	if got := d.Dialect(); got != engine.DialectPostgres {
		t.Errorf("Dialect() = %q, want %q", got, engine.DialectPostgres)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("yugabyte")
	if !ok {
		t.Fatal("yugabyte not registered")
	}
	if caps.Engine.ID != "yugabyte" || caps.Engine.DisplayName != "YugabyteDB" || caps.Engine.Dialect != engine.DialectPostgres {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	postgresCaps, _ := engine.Describe("postgres")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLClassify, engine.CapabilitySQLComplete, engine.CapabilitySQLSafetyCheck,
		engine.CapabilitySQLExplain, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != postgresCaps.Capabilities[capID] {
			t.Errorf("capability %q: yugabyte=%v postgres=%v", capID, caps.Capabilities[capID], postgresCaps.Capabilities[capID])
		}
	}
}

func TestConnect(t *testing.T) {
	d := connect(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestMalformedDSNRejected(t *testing.T) {
	d := &driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: "not a dsn", Driver: "yugabyte"}); err == nil {
		t.Fatal("expected Connect to reject a malformed DSN")
	}
}

func TestInspectDirectoryReportsTable(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	exec := func(stmt string) {
		t.Helper()
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	exec("DROP TABLE IF EXISTS yb_directory_test")
	t.Cleanup(func() { exec("DROP TABLE IF EXISTS yb_directory_test") })
	exec("CREATE TABLE yb_directory_test (id INT PRIMARY KEY)")

	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "yugabyte"}).
		Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	if !directoryHasRef(directory, metadata.ObjectRef{Scope: scope, Kind: "table", Name: "yb_directory_test"}) {
		t.Fatalf("directory missing yb_directory_test: %+v", directory.Roots)
	}
}

func TestConnectionContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := &driver{}
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "yugabyte"}); err != nil {
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
