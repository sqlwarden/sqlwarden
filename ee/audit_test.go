package ee

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/audit/audittest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
)

func TestEnterpriseAuditWriterContract(t *testing.T) {
	audittest.Run(t, func(t *testing.T) audittest.Subject {
		writer, _, store := newAuditFixture(t, AuditOptions{})
		return audittest.Subject{
			Writer: writer,
			Recorded: func(ctx context.Context) ([]audit.Event, error) {
				return store.Events(ctx, 0)
			},
		}
	})
}

func TestAuditPipelineOrderIsDurabilityFirst(t *testing.T) {
	writer, _, _ := newAuditFixture(t, AuditOptions{})

	var names []string
	var fatal []bool
	for _, stage := range writer.pipeline() {
		names = append(names, stage.name)
		fatal = append(fatal, stage.fatal)
	}

	wantNames := []string{stageCoreWrite, stageHashChain, stageRetentionLock, stageSigning, stageExport}
	if len(names) != len(wantNames) {
		t.Fatalf("pipeline = %v, want %v", names, wantNames)
	}
	for i, want := range wantNames {
		if names[i] != want {
			t.Fatalf("pipeline = %v, want %v", names, wantNames)
		}
	}
	// Only durability may fail a use case. Every later stage works from a
	// record that is already durable, so its failure is reconciled rather than
	// reported as the mutation having failed.
	wantFatal := []bool{true, false, false, false, false}
	for i, want := range wantFatal {
		if fatal[i] != want {
			t.Fatalf("stage %q fatal = %t, want %t", names[i], fatal[i], want)
		}
	}
}

func TestCoreWriteFailureFailsTheUseCase(t *testing.T) {
	writeErr := errors.New("audit storage unavailable")
	db := newMigratedDB(t)
	deps := newAuditDeps(db, audit.NewSQLStore(db.DB))
	core := audit.WriterFunc(func(context.Context, audit.Event) error { return writeErr })
	writer := newAuditWriter(core, deps, AuditOptions{})

	err := writer.Write(context.Background(), successEvent("access.role.created"))
	if !errors.Is(err, writeErr) {
		t.Fatalf("write error = %v, want %v", err, writeErr)
	}
}

func TestExporterSeesACompleteSeal(t *testing.T) {
	exporter := &recordingExporter{}
	writer, _, _ := newAuditFixture(t, AuditOptions{Signer: staticSigner{}, Exporter: exporter})

	if err := writer.Write(context.Background(), successEvent("access.policy.granted")); err != nil {
		t.Fatal(err)
	}
	if len(exporter.seals) != 1 {
		t.Fatalf("exported %d events, want 1", len(exporter.seals))
	}
	seal := exporter.seals[0]
	if seal.Index != 1 || seal.Hash == "" {
		t.Errorf("seal = %+v, want a chained first record", seal)
	}
	if seal.RetainUntil == nil {
		t.Error("seal has no retention lock, so export ran before the lock was applied")
	}
	if seal.Signature != "signature-1" || seal.SigningKeyID != "test-key" {
		t.Errorf("seal signature = %q/%q, want the signing hook output", seal.Signature, seal.SigningKeyID)
	}
}

func TestHashChainIsDeterministicAndVerifiable(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	for i := range 3 {
		if err := writer.Write(ctx, successEvent("access.role.created."+string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}

	verified, err := VerifyAuditChain(ctx, db, events)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified != 3 {
		t.Fatalf("verified %d records, want 3", verified)
	}

	records, err := writer.store.chainRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := events.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	previous := ""
	for i, record := range records {
		if record.PreviousHash != previous {
			t.Fatalf("record %d does not follow its predecessor", i)
		}
		if got := chainHash(previous, record.Index, recorded[i]); got != record.Hash {
			t.Fatalf("recomputed hash %q, want %q", got, record.Hash)
		}
		if again := chainHash(previous, record.Index, recorded[i]); again != record.Hash {
			t.Fatal("hash is not deterministic for identical input")
		}
		previous = record.Hash
	}
}

func TestVerificationDetectsAnEditedAuditRecord(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	for _, action := range []string{"access.role.created", "access.role.deleted"} {
		if err := writer.Write(ctx, successEvent(action)); err != nil {
			t.Fatal(err)
		}
	}

	_, err := db.NewUpdate().
		TableExpr("audit_events").
		Set("outcome = ?", audit.OutcomeDenied).
		Where("action = ?", "access.role.deleted").
		Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	verified, err := VerifyAuditChain(ctx, db, events)
	if err == nil {
		t.Fatal("verification accepted an edited audit record")
	}
	if verified != 1 {
		t.Fatalf("verification stopped at %d, want the first record to still verify", verified)
	}
}

// TestVerificationDetectsAnUnchainedAuditEvent covers an event inserted into
// the durable trail without evidence. No per-record check can see it, because
// there is no record to check.
func TestVerificationDetectsAnUnchainedAuditEvent(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	if err := writer.Write(ctx, successEvent("access.role.created")); err != nil {
		t.Fatal(err)
	}

	smuggled := map[string]any{
		"id":          database.NewID(),
		"occurred_at": time.Now().UTC(),
		"action":      "access.policy.granted",
		"resource":    "",
		"resource_id": "",
		"outcome":     audit.OutcomeSuccess,
		"metadata":    "{}",
	}
	if _, err := db.NewInsert().TableExpr("audit_events").Model(&smuggled).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyAuditChain(ctx, db, events); err == nil {
		t.Fatal("verification accepted a durable audit event that no chain record covers")
	}
}

// TestVerificationDetectsATruncatedChainTail covers evidence removed from the
// end of the chain, where every surviving record still links correctly.
func TestVerificationDetectsATruncatedChainTail(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	for _, action := range []string{"access.role.created", "access.role.deleted"} {
		if err := writer.Write(ctx, successEvent(action)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.NewDelete().TableExpr("ee_audit_chain_records").Where("chain_index = ?", 2).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	verified, err := VerifyAuditChain(ctx, db, events)
	if err == nil {
		t.Fatal("verification accepted a truncated chain tail")
	}
	if verified != 1 {
		t.Fatalf("verified %d records, want the surviving record to be counted", verified)
	}
}

// TestVerificationDetectsAHeadMismatch covers chain state rewritten to point
// somewhere other than the last record.
func TestVerificationDetectsAHeadMismatch(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	if err := writer.Write(ctx, successEvent("access.role.created")); err != nil {
		t.Fatal(err)
	}

	_, err := db.NewUpdate().
		TableExpr("ee_audit_chain_state").
		Set("last_hash = ?", "0000000000000000000000000000000000000000000000000000000000000000").
		Where("scope = ?", auditChainScope).
		Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyAuditChain(ctx, db, events); err == nil {
		t.Fatal("verification accepted a chain head that does not match its last record")
	}
}

func TestRetentionLockPrefersTheOrganizationPolicy(t *testing.T) {
	writer, db, _ := newAuditFixture(t, AuditOptions{DefaultRetention: time.Hour})
	ctx := context.Background()

	for _, lock := range []map[string]any{
		{"org_id": nil, "retain_seconds": int64(7200)},
		{"org_id": int64(42), "retain_seconds": int64(10800)},
	} {
		if _, err := db.NewInsert().TableExpr("ee_audit_retention_locks").Model(&lock).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	orgID := int64(42)
	event := successEvent("access.policy.granted")
	event.OrgID = &orgID
	event.OccurredAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := writer.Write(ctx, event); err != nil {
		t.Fatal(err)
	}

	record, found, err := writer.store.chainRecord(ctx, event.ID)
	if err != nil || !found {
		t.Fatalf("chain record: found=%t err=%v", found, err)
	}
	want := event.OccurredAt.Add(3 * time.Hour)
	if record.RetainUntil == nil || !record.RetainUntil.UTC().Equal(want) {
		t.Fatalf("retain until = %v, want %s from the organization lock", record.RetainUntil, want)
	}
}

func TestRetentionLockFallsBackToTheConfiguredDefault(t *testing.T) {
	writer, _, _ := newAuditFixture(t, AuditOptions{DefaultRetention: 48 * time.Hour})
	ctx := context.Background()

	event := successEvent("access.policy.revoked")
	event.OccurredAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := writer.Write(ctx, event); err != nil {
		t.Fatal(err)
	}

	record, found, err := writer.store.chainRecord(ctx, event.ID)
	if err != nil || !found {
		t.Fatalf("chain record: found=%t err=%v", found, err)
	}
	want := event.OccurredAt.Add(48 * time.Hour)
	if record.RetainUntil == nil || !record.RetainUntil.UTC().Equal(want) {
		t.Fatalf("retain until = %v, want %s", record.RetainUntil, want)
	}
}

// TestRetentionPolicyIsUniquePerScope covers the schema rule that one scope has
// one lock: a second instance-wide row would make the retention a database
// ordering accident rather than a policy.
func TestRetentionPolicyIsUniquePerScope(t *testing.T) {
	db := newMigratedDB(t)
	ctx := context.Background()

	for _, lock := range []map[string]any{
		{"org_id": nil, "retain_seconds": int64(7200)},
		{"org_id": int64(42), "retain_seconds": int64(10800)},
	} {
		if _, err := db.NewInsert().TableExpr("ee_audit_retention_locks").Model(&lock).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	for name, duplicate := range map[string]map[string]any{
		"instance wide": {"org_id": nil, "retain_seconds": int64(60)},
		"organization":  {"org_id": int64(42), "retain_seconds": int64(60)},
	} {
		t.Run("rejects a second "+name+" lock", func(t *testing.T) {
			row := duplicate
			if _, err := db.NewInsert().TableExpr("ee_audit_retention_locks").Model(&row).Exec(ctx); err == nil {
				t.Fatal("inserted a duplicate retention lock")
			}
		})
	}
}

// TestSigningFailureIsReconciledWithoutFailingTheUseCase covers the central
// rule: the audit record is durable before signing runs, so a signing outage
// degrades evidence rather than failing a mutation that already committed.
func TestSigningFailureIsReconciledWithoutFailingTheUseCase(t *testing.T) {
	signer := &flakySigner{err: errors.New("hsm unavailable")}
	writer, db, events := newAuditFixture(t, AuditOptions{Signer: signer})
	ctx := context.Background()

	first := successEvent("access.role.created")
	if err := writer.Write(ctx, first); err != nil {
		t.Fatalf("a signing outage failed a committed mutation: %v", err)
	}

	recorded, err := events.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("recorded %d events, want the durable record to survive a signing failure", len(recorded))
	}
	if _, found, err := writer.store.chainRecord(ctx, first.ID); err != nil || !found {
		t.Fatalf("event was not chained before signing ran: found=%t err=%v", found, err)
	}

	signer.err = nil
	if err := writer.Write(ctx, successEvent("access.role.deleted")); err != nil {
		t.Fatal(err)
	}

	record, _, err := writer.store.chainRecord(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Signature == "" {
		t.Fatal("the backlogged record was never signed once the signer recovered")
	}
	if _, err := VerifyAuditChain(ctx, db, events); err != nil {
		t.Fatalf("verify after reconciliation: %v", err)
	}
}

// TestChainFailureIsReconciledOnALaterWrite covers a chain outage: the durable
// record is left unchained, the use case still succeeds, and the next write
// seals the backlog in order.
func TestChainFailureIsReconciledOnALaterWrite(t *testing.T) {
	writer, db, events := newAuditFixture(t, AuditOptions{})
	ctx := context.Background()

	// A missing chain table is the bluntest chain outage available, and it
	// fails the stage the same way a storage outage would.
	if _, err := db.ExecContext(ctx, "ALTER TABLE ee_audit_chain_records RENAME TO ee_audit_chain_records_offline"); err != nil {
		t.Fatal(err)
	}

	unchained := successEvent("access.role.created")
	if err := writer.Write(ctx, unchained); err != nil {
		t.Fatalf("a chain outage failed a committed mutation: %v", err)
	}
	recorded, err := events.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("recorded %d events, want the record to be durable despite the chain outage", len(recorded))
	}

	if _, err := db.ExecContext(ctx, "ALTER TABLE ee_audit_chain_records_offline RENAME TO ee_audit_chain_records"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(ctx, successEvent("access.role.deleted")); err != nil {
		t.Fatal(err)
	}

	record, found, err := writer.store.chainRecord(ctx, unchained.ID)
	if err != nil || !found {
		t.Fatalf("backlogged event was not chained after recovery: found=%t err=%v", found, err)
	}
	if record.Index != 1 {
		t.Fatalf("backlogged event chained at index %d, want the chain to stay in order", record.Index)
	}
	if _, err := VerifyAuditChain(ctx, db, events); err != nil {
		t.Fatalf("verify after reconciliation: %v", err)
	}
}

// TestChainAppendRetriesAfterAConflict covers another process appending
// between this writer reading the head and committing its own record.
func TestChainAppendRetriesAfterAConflict(t *testing.T) {
	db := newMigratedDB(t)
	store := audit.NewSQLStore(db.DB)
	ctx := context.Background()

	var once sync.Once
	deps := newAuditDeps(db, store)
	interfering := deps
	// The clock runs after the head is read and before the append commits,
	// which is exactly the window a competing appender has to win.
	interfering.Now = func() time.Time {
		once.Do(func() { appendCompetingEvent(t, db, store) })
		return time.Now()
	}
	writer := newAuditWriter(audit.NewCoreWriter(store, deps.Now), interfering, AuditOptions{})

	event := successEvent("access.role.created")
	event.OccurredAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := writer.Write(ctx, event); err != nil {
		t.Fatal(err)
	}

	record, found, err := writer.store.chainRecord(ctx, event.ID)
	if err != nil || !found {
		t.Fatalf("chain record: found=%t err=%v", found, err)
	}
	if record.Index != 2 {
		t.Fatalf("chain index = %d, want the retry to append after the competing record", record.Index)
	}
	if _, err := VerifyAuditChain(ctx, db, store); err != nil {
		t.Fatalf("verify after a contended append: %v", err)
	}
}

func TestExportFailureKeepsTheAuditEventAndRecordsTheCursor(t *testing.T) {
	exporter := &recordingExporter{err: errors.New("siem unreachable at https://siem.example/ingest?token=secret")}
	writer, _, events := newAuditFixture(t, AuditOptions{Exporter: exporter})
	ctx := context.Background()

	event := successEvent("access.policy.granted")
	if err := writer.Write(ctx, event); err != nil {
		t.Fatalf("export failure was reported to the use case: %v", err)
	}

	recorded, err := events.Events(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].ID != event.ID {
		t.Fatalf("recorded %d events, want the durable record to survive an export failure", len(recorded))
	}
	if _, found, err := writer.store.chainRecord(ctx, event.ID); err != nil || !found {
		t.Fatalf("chain record missing after export failure: found=%t err=%v", found, err)
	}

	cursor, found, err := writer.store.exportCursor(ctx, exporter.Name())
	if err != nil || !found {
		t.Fatalf("export cursor: found=%t err=%v", found, err)
	}
	if cursor.PendingEventID != event.ID || cursor.PendingIndex != 1 {
		t.Errorf("pending delivery = %q/%d, want %q/1", cursor.PendingEventID, cursor.PendingIndex, event.ID)
	}
	if cursor.FailureCount != 1 {
		t.Errorf("failure count = %d, want 1", cursor.FailureCount)
	}
	if cursor.DeliveredIndex != 0 {
		t.Errorf("delivered index = %d, want 0 for an undelivered event", cursor.DeliveredIndex)
	}
	// The exporter's own error text can name endpoints and credentials, so the
	// cursor keeps only a bounded class.
	if cursor.LastErrorClass != exportFailureRejected {
		t.Errorf("last error class = %q, want %q", cursor.LastErrorClass, exportFailureRejected)
	}
}

// TestExportBacklogIsDeliveredInOrderAfterRecovery covers the meaning of the
// cursor: a delivered position claims every event up to it reached the
// exporter, so recovery has to replay the backlog rather than skip it.
func TestExportBacklogIsDeliveredInOrderAfterRecovery(t *testing.T) {
	exporter := &recordingExporter{err: errors.New("siem unreachable")}
	writer, _, _ := newAuditFixture(t, AuditOptions{Exporter: exporter})
	ctx := context.Background()

	first := successEvent("access.role.created")
	if err := writer.Write(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := successEvent("access.role.deleted")
	if err := writer.Write(ctx, second); err != nil {
		t.Fatal(err)
	}

	cursor, _, err := writer.store.exportCursor(ctx, exporter.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cursor.PendingEventID != first.ID || cursor.PendingIndex != 1 {
		t.Fatalf("pending delivery = %q/%d, want the oldest undelivered event %q/1", cursor.PendingEventID, cursor.PendingIndex, first.ID)
	}
	if cursor.FailureCount != 2 {
		t.Fatalf("failure count = %d, want 2", cursor.FailureCount)
	}
	if cursor.DeliveredIndex != 0 {
		t.Fatalf("delivered index = %d, want 0 while nothing has been delivered", cursor.DeliveredIndex)
	}

	exporter.err = nil
	third := successEvent("access.policy.granted")
	if err := writer.Write(ctx, third); err != nil {
		t.Fatal(err)
	}

	wantDelivered := []string{first.ID, second.ID, third.ID}
	if len(exporter.events) != len(wantDelivered) {
		t.Fatalf("exporter received %d events, want the backlog replayed in order", len(exporter.events))
	}
	for i, want := range wantDelivered {
		if exporter.events[i].ID != want {
			t.Fatalf("delivered event %d = %q, want %q", i, exporter.events[i].ID, want)
		}
	}

	cursor, _, err = writer.store.exportCursor(ctx, exporter.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cursor.PendingEventID != "" || cursor.PendingIndex != 0 || cursor.FailureCount != 0 {
		t.Fatalf("cursor after delivery = %+v, want a cleared backlog", cursor)
	}
	if cursor.DeliveredEventID != third.ID || cursor.DeliveredIndex != 3 {
		t.Fatalf("delivered = %q/%d, want %q/3", cursor.DeliveredEventID, cursor.DeliveredIndex, third.ID)
	}
}

// TestDeliveredCursorNeverMovesBackward covers a late or duplicated
// confirmation for an event the exporter already acknowledged.
func TestDeliveredCursorNeverMovesBackward(t *testing.T) {
	db := newMigratedDB(t)
	store := &auditStore{db: db.DB}
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.recordDelivery(ctx, "test-siem", "event-3", 3, now); err != nil {
		t.Fatal(err)
	}
	if err := store.recordDelivery(ctx, "test-siem", "event-2", 2, now); err != nil {
		t.Fatal(err)
	}

	cursor, found, err := store.exportCursor(ctx, "test-siem")
	if err != nil || !found {
		t.Fatalf("export cursor: found=%t err=%v", found, err)
	}
	if cursor.DeliveredIndex != 3 || cursor.DeliveredEventID != "event-3" {
		t.Fatalf("delivered = %q/%d, want the cursor to stay at event-3/3", cursor.DeliveredEventID, cursor.DeliveredIndex)
	}
}

// TestDeliveryFailureKeepsTheOldestBacklog covers a failure recorded for a
// later event never replacing an older undelivered one. Delivery always
// resumes at the delivered position, so the cursor keeps counting attempts
// against the event that is actually blocking.
func TestDeliveryFailureKeepsTheOldestBacklog(t *testing.T) {
	db := newMigratedDB(t)
	store := &auditStore{db: db.DB}
	ctx := context.Background()
	now := time.Now().UTC()

	for range 3 {
		if err := store.recordDeliveryFailure(ctx, "test-siem", "event-1", 1, exportFailureTimeout, now); err != nil {
			t.Fatal(err)
		}
	}

	cursor, _, err := store.exportCursor(ctx, "test-siem")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.PendingEventID != "event-1" || cursor.PendingIndex != 1 {
		t.Fatalf("pending = %q/%d, want event-1/1", cursor.PendingEventID, cursor.PendingIndex)
	}
	if cursor.FailureCount != 3 {
		t.Fatalf("failure count = %d, want 3", cursor.FailureCount)
	}
	if cursor.DeliveredIndex != 0 {
		t.Fatalf("delivered index = %d, want an untouched delivered position", cursor.DeliveredIndex)
	}
}

func TestExportFailureClassIsBounded(t *testing.T) {
	for name, testCase := range map[string]struct {
		err  error
		want string
	}{
		"canceled":     {err: context.Canceled, want: exportFailureCanceled},
		"timed out":    {err: context.DeadlineExceeded, want: exportFailureTimeout},
		"vendor error": {err: errors.New("401 from https://siem.example?token=secret"), want: exportFailureRejected},
	} {
		t.Run(name, func(t *testing.T) {
			if got := exportFailureClass(testCase.err); got != testCase.want {
				t.Fatalf("class = %q, want %q", got, testCase.want)
			}
		})
	}
}

func successEvent(action string) audit.Event {
	return audit.Event{ID: database.NewID(), Action: action, Outcome: audit.OutcomeSuccess}
}

// appendCompetingEvent chains one event the way another process would, so the
// head moves underneath a writer that has already read it.
func appendCompetingEvent(t *testing.T, db *database.DB, store *audit.SQLStore) {
	t.Helper()
	ctx := context.Background()

	event := audit.Normalize(successEvent("access.policy.granted"), time.Now)
	if err := store.InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}

	competitor := &auditStore{db: db.DB}
	head, err := competitor.head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record := auditChainRecord{
		EventID:      event.ID,
		Index:        head.Index + 1,
		PreviousHash: head.Hash,
		Hash:         chainHash(head.Hash, head.Index+1, event),
		CreatedAt:    time.Now().UTC(),
	}
	if err := competitor.appendRecord(ctx, record, head.Index); err != nil {
		t.Fatal(err)
	}
}

func newAuditDeps(db *database.DB, events audit.Reader) edition.Dependencies {
	return edition.Dependencies{
		DB:          db,
		AuditEvents: events,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}.Normalize()
}

func newAuditFixture(t *testing.T, options AuditOptions) (*auditWriter, *database.DB, *audit.SQLStore) {
	t.Helper()

	db := newMigratedDB(t)
	store := audit.NewSQLStore(db.DB)
	deps := newAuditDeps(db, store)
	writer := newAuditWriter(audit.NewCoreWriter(store, deps.Now), deps, options)
	return writer, db, store
}

// newMigratedDB returns a database with core and Enterprise migrations
// applied, which is what an Enterprise decorator expects at runtime.
func newMigratedDB(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "ee.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}

	candidate, err := New(enterpriseConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := edition.Migrate(context.Background(), candidate, db); err != nil {
		t.Fatal(err)
	}
	return db
}

type recordingExporter struct {
	err    error
	events []audit.Event
	seals  []Seal
}

func (e *recordingExporter) Name() string { return "test-siem" }

func (e *recordingExporter) Export(_ context.Context, event audit.Event, seal Seal) error {
	if e.err != nil {
		return e.err
	}
	e.events = append(e.events, event)
	e.seals = append(e.seals, seal)
	return nil
}

type staticSigner struct{}

func (staticSigner) Sign(context.Context, string) (string, string, error) {
	return "signature-1", "test-key", nil
}

// flakySigner fails while err is set and signs once it is cleared.
type flakySigner struct{ err error }

func (s *flakySigner) Sign(context.Context, string) (string, string, error) {
	if s.err != nil {
		return "", "", s.err
	}
	return "signature-1", "test-key", nil
}
