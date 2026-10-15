//go:build enterprise

package ee

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
)

func enterpriseAuditDB(t *testing.T) (*database.DB, *audit.SQLStore) {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "audit.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	composition, err := edition.Compose(context.Background(), New(), edition.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if err := edition.Migrate(context.Background(), db, composition.Migrations()); err != nil {
		t.Fatal(err)
	}
	return db, audit.NewSQLStore(db.DB)
}

type flakySigner struct {
	mu       sync.Mutex
	failures int
}

func (s *flakySigner) Sign(_ context.Context, hash string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures > 0 {
		s.failures--
		return "", "", errors.New("signer unavailable")
	}
	return "signature:" + hash, "key-1", nil
}

func auditEvent(id string) audit.Event {
	return audit.Event{ID: id, Action: "test.action", Resource: "test", ResourceID: id, Outcome: audit.OutcomeSuccess}
}

func TestAuditIsDurableBeforeOptionalEvidenceAndReconciles(t *testing.T) {
	db, store := enterpriseAuditDB(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	signer := &flakySigner{failures: 1}
	writer := NewAuditWriter(audit.NewCoreWriter(store, func() time.Time { return now }), edition.Dependencies{
		SQL: db.DB, AuditEvents: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now },
	}, AuditOptions{Signer: signer})

	if err := writer.Write(context.Background(), auditEvent("event-1")); err != nil {
		t.Fatalf("post-durability signer failure escaped: %v", err)
	}
	events, err := store.Events(context.Background(), 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("durable events = %d, err = %v", len(events), err)
	}
	var records int
	if err := db.NewSelect().TableExpr("ee_audit_chain_records").ColumnExpr("count(*)").Scan(context.Background(), &records); err != nil {
		t.Fatal(err)
	}
	if records != 0 {
		t.Fatalf("records after failed signer = %d", records)
	}

	if err := writer.Write(context.Background(), auditEvent("event-2")); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAuditChain(context.Background(), store, db.DB); err != nil {
		t.Fatal(err)
	}
	if err := db.NewSelect().TableExpr("ee_audit_chain_records").ColumnExpr("count(*)").Scan(context.Background(), &records); err != nil {
		t.Fatal(err)
	}
	if records != 2 {
		t.Fatalf("reconciled records = %d, want 2", records)
	}
	seal, err := writer.record(context.Background(), "event-1")
	if err != nil {
		t.Fatal(err)
	}
	if seal.Signature == "" || seal.RetainUntil == nil || !seal.RetainUntil.Equal(now.Add(defaultRetention)) {
		t.Fatalf("seal = %+v", seal)
	}
	if _, err := db.NewUpdate().TableExpr("ee_audit_chain_records").Set("hash = ?", "tampered").Where("event_id = ?", "event-1").Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAuditChain(context.Background(), store, db.DB); err == nil {
		t.Fatal("tampered chain verified successfully")
	}
}

func TestAuditRejectsInvalidEventBeforeCoreWrite(t *testing.T) {
	called := false
	writer := NewAuditWriter(audit.WriterFunc(func(context.Context, audit.Event) error {
		called = true
		return nil
	}), edition.Dependencies{}, AuditOptions{})
	if err := writer.Write(context.Background(), audit.Event{}); !errors.Is(err, audit.ErrInvalidEvent) {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("invalid event reached core writer")
	}
}

type flakyExporter struct {
	mu       sync.Mutex
	failures int
	attempts []string
}

func (*flakyExporter) Name() string { return "test-exporter" }
func (e *flakyExporter) Export(_ context.Context, event audit.Event, _ Seal) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attempts = append(e.attempts, event.ID)
	if e.failures > 0 {
		e.failures--
		return errors.New("vendor detail must not persist")
	}
	return nil
}

func TestExporterRetriesContiguouslyFromDurableEvents(t *testing.T) {
	db, store := enterpriseAuditDB(t)
	exporter := &flakyExporter{failures: 1}
	writer := NewAuditWriter(audit.NewCoreWriter(store, time.Now), edition.Dependencies{
		SQL: db.DB, AuditEvents: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, AuditOptions{Exporter: exporter})
	if err := writer.Write(context.Background(), auditEvent("event-1")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(context.Background(), auditEvent("event-2")); err != nil {
		t.Fatal(err)
	}
	exporter.mu.Lock()
	attempts := append([]string(nil), exporter.attempts...)
	exporter.mu.Unlock()
	want := []string{"event-1", "event-1", "event-2"}
	if fmt.Sprint(attempts) != fmt.Sprint(want) {
		t.Fatalf("export attempts = %v, want %v", attempts, want)
	}
	var cursor struct {
		DeliveredEventID string `bun:"delivered_event_id"`
		FailureCount     int64  `bun:"failure_count"`
		LastErrorClass   string `bun:"last_error_class"`
	}
	if err := db.NewSelect().TableExpr("ee_audit_export_cursors").ColumnExpr("delivered_event_id, failure_count, last_error_class").Where("exporter = ?", exporter.Name()).Scan(context.Background(), &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor.DeliveredEventID != "event-2" || cursor.FailureCount != 0 || cursor.LastErrorClass != "" {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestConcurrentAuditAppendsRemainVerifiable(t *testing.T) {
	db, store := enterpriseAuditDB(t)
	writer := NewAuditWriter(audit.NewCoreWriter(store, time.Now), edition.Dependencies{
		SQL: db.DB, AuditEvents: store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, AuditOptions{})
	var group sync.WaitGroup
	for index := range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := writer.Write(context.Background(), auditEvent(fmt.Sprintf("event-%d", index))); err != nil {
				t.Errorf("Write: %v", err)
			}
		}()
	}
	group.Wait()
	if err := VerifyAuditChain(context.Background(), store, db.DB); err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseMigrationOwnsOnlyPrefixedTablesAndLedger(t *testing.T) {
	db, _ := enterpriseAuditDB(t)
	composition, err := edition.Compose(context.Background(), New(), edition.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if err := edition.Migrate(context.Background(), db, composition.Migrations()); err != nil {
		t.Fatalf("idempotent Enterprise migration: %v", err)
	}
	var tables []string
	if err := db.NewRaw("SELECT name FROM sqlite_master WHERE type = 'table' AND (name LIKE 'ee_%' OR name = 'schema_migrations_ee') ORDER BY name").Scan(context.Background(), &tables); err != nil {
		t.Fatal(err)
	}
	want := []string{"ee_audit_chain_records", "ee_audit_chain_state", "ee_audit_export_cursors", "ee_audit_retention_locks", "schema_migrations_ee"}
	if fmt.Sprint(tables) != fmt.Sprint(want) {
		t.Fatalf("enterprise tables = %v, want %v", tables, want)
	}
}

func TestExportFailureClassIsBounded(t *testing.T) {
	for err, want := range map[error]string{
		context.Canceled: "canceled",
		fmt.Errorf("send: %w", context.DeadlineExceeded): "timeout",
		errors.New("collector said: secret detail"):      "rejected",
	} {
		if got := exportFailureClass(err); got != want {
			t.Errorf("exportFailureClass(%v) = %q, want %q", err, got, want)
		}
	}
}
