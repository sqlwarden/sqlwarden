package neon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestRegistered(t *testing.T) {
	d, err := engine.New("neon")
	if err != nil {
		t.Fatalf("engine.New(neon): %v", err)
	}
	if _, ok := d.(*driver); !ok {
		t.Fatalf("engine.New(neon) returned %T, want *driver", d)
	}
}

func TestDescribeReportsOwnIdentityAndInheritedCapabilities(t *testing.T) {
	caps, ok := engine.Describe("neon")
	if !ok {
		t.Fatal("neon not registered")
	}
	if caps.Engine.ID != "neon" || caps.Engine.DisplayName != "Neon" || caps.Engine.Dialect != engine.DialectPostgres {
		t.Fatalf("unexpected identity: %+v", caps.Engine)
	}
	postgresCaps, _ := engine.Describe("postgres")
	for _, capID := range []engine.Capability{
		engine.CapabilitySchemaDirectory, engine.CapabilitySchemaObjects, engine.CapabilityDDL,
		engine.CapabilitySQLClassify, engine.CapabilitySQLComplete, engine.CapabilitySQLSafetyCheck,
		engine.CapabilitySQLExplain, engine.CapabilityTLS, engine.CapabilitySSHTunnel,
	} {
		if caps.Capabilities[capID] != postgresCaps.Capabilities[capID] {
			t.Errorf("capability %q: neon=%v postgres=%v", capID, caps.Capabilities[capID], postgresCaps.Capabilities[capID])
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
	d := &driver{}
	ctx := context.Background()
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "neon"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConnectInvalidDSNFailsWithoutRetrying(t *testing.T) {
	d := &driver{}
	start := time.Now()
	err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: "not a valid dsn", Driver: "neon"})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected Connect to fail for a malformed DSN")
	}
	if elapsed > time.Second {
		t.Fatalf("malformed DSN took %s to fail; expected immediate failure with no cold-start retry", elapsed)
	}
}

func TestConnectInheritsSchemaSelection(t *testing.T) {
	d := &driver{}
	ctx := context.Background()
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "neon"}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if _, err := d.Execute(ctx, `CREATE SCHEMA IF NOT EXISTS neon_scope`); err != nil {
		t.Fatal(err)
	}

	scoped := &driver{}
	scope := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: "testdb"},
		metadata.ScopeSegment{Kind: "schema", Name: "neon_scope"},
	)
	if err := scoped.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "neon", DefaultScope: scope}); err != nil {
		t.Fatal(err)
	}
	defer scoped.Close()

	rs, err := scoped.Query(ctx, `SELECT current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	if got := rs.Rows[0][0].Text; got != "neon_scope" {
		t.Fatalf("current_schema() = %q, want neon_scope", got)
	}
}

func TestConnectWithColdStartRetrySucceedsAfterTransientFailures(t *testing.T) {
	orig := coldStartInitialBackoff
	coldStartInitialBackoff = time.Millisecond
	t.Cleanup(func() { coldStartInitialBackoff = orig })

	attempts := 0
	err := connectWithColdStartRetry(context.Background(), engine.ConnectionConfig{}, func(context.Context, engine.ConnectionConfig) error {
		attempts++
		if attempts < 3 {
			return errors.New("compute waking up")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestConnectWithColdStartRetryExhaustsAndReturnsLastError(t *testing.T) {
	orig := coldStartInitialBackoff
	coldStartInitialBackoff = time.Millisecond
	t.Cleanup(func() { coldStartInitialBackoff = orig })

	attempts := 0
	wantErr := errors.New("compute still suspended")
	err := connectWithColdStartRetry(context.Background(), engine.ConnectionConfig{}, func(context.Context, engine.ConnectionConfig) error {
		attempts++
		return wantErr
	})
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
	if attempts != coldStartRetries+1 {
		t.Fatalf("attempts = %d, want %d", attempts, coldStartRetries+1)
	}
}

func TestConnectWithColdStartRetryDoesNotRetryOnDefinitivePgError(t *testing.T) {
	orig := coldStartInitialBackoff
	coldStartInitialBackoff = time.Hour
	t.Cleanup(func() { coldStartInitialBackoff = orig })

	attempts := 0
	pgErr := &pgconn.PgError{Code: "3D000", Message: `database "missing" does not exist`}
	err := connectWithColdStartRetry(context.Background(), engine.ConnectionConfig{}, func(context.Context, engine.ConnectionConfig) error {
		attempts++
		return pgErr
	})
	if !errors.Is(err, pgErr) {
		t.Fatalf("expected wrapped %v, got %v", pgErr, err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (a PgError is a definitive server response, not a cold-start symptom)", attempts)
	}
}

func TestConnectFailsFastOnUnknownDatabase(t *testing.T) {
	orig := coldStartInitialBackoff
	coldStartInitialBackoff = time.Hour
	t.Cleanup(func() { coldStartInitialBackoff = orig })

	badDSN := testDSN[:strings.LastIndex(testDSN, "/")] + "/does_not_exist"
	d := &driver{}
	start := time.Now()
	err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: badDSN, Driver: "neon"})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected Connect to fail for a nonexistent database")
	}
	if elapsed > time.Second {
		t.Fatalf("nonexistent database took %s to fail; expected immediate failure with no cold-start retry", elapsed)
	}
}

func TestConnectWithColdStartRetryStopsOnContextCancellation(t *testing.T) {
	orig := coldStartInitialBackoff
	coldStartInitialBackoff = time.Hour
	t.Cleanup(func() { coldStartInitialBackoff = orig })

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	err := connectWithColdStartRetry(ctx, engine.ConnectionConfig{}, func(context.Context, engine.ConnectionConfig) error {
		attempts++
		return errors.New("compute waking up")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
