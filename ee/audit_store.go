package ee

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/uptrace/bun"
)

// auditChainScope names the single append-only hash chain an instance keeps.
// Chain state is stored per scope so a later deployment can shard the chain
// without changing the record shape.
const auditChainScope = "instance"

// errChainConflict reports a concurrent append against the same chain head.
// The append is refused rather than applied out of order, because an audit
// chain that skips or reorders an index is no longer tamper evident. The
// caller re-reads the head and retries a bounded number of times.
var errChainConflict = errors.New("audit chain head changed during append")

// auditStore owns the Enterprise audit tables: chain state and records,
// retention locks, and export cursors. Core audit rows are read and written
// through core contracts, never through this store.
type auditStore struct {
	db bun.IDB
}

func newAuditStore(db *database.DB) *auditStore {
	if db == nil {
		return nil
	}
	return &auditStore{db: db.DB}
}

// chainHead is the position the next append builds on. EventID is the core
// audit event the last record covers, which is what verification checks the
// head against.
type chainHead struct {
	Index   int64  `bun:"last_index"`
	Hash    string `bun:"last_hash"`
	EventID string `bun:"last_event_id"`
}

// ensureChainState creates the chain row if it does not exist yet. Bootstrap
// is an idempotent upsert rather than a read-then-insert, so two processes
// starting at the same time cannot both try to create it.
func (s *auditStore) ensureChainState(ctx context.Context, exec bun.IDB) error {
	state := map[string]any{
		"scope":           auditChainScope,
		"last_index":      int64(0),
		"last_hash":       "",
		"last_event_id":   "",
		"sealed_event_id": "",
		"updated_at":      time.Now().UTC(),
	}
	_, err := exec.NewInsert().
		TableExpr("ee_audit_chain_state").
		Model(&state).
		On("CONFLICT (scope) DO NOTHING").
		Exec(ctx)
	return err
}

// head returns the current chain head, treating a missing chain row as an
// empty chain.
func (s *auditStore) head(ctx context.Context) (chainHead, error) {
	var head chainHead
	err := s.db.NewSelect().
		TableExpr("ee_audit_chain_state").
		ColumnExpr("last_index, last_hash, last_event_id").
		Where("scope = ?", auditChainScope).
		Limit(1).
		Scan(ctx, &head)
	if errors.Is(err, sql.ErrNoRows) {
		return chainHead{}, nil
	}
	if err != nil {
		return chainHead{}, err
	}
	return head, nil
}

// sealWatermark returns the reconciliation resume point: every durable audit
// event at or before it has complete evidence. It is deliberately not the
// chain head. Two processes can normalize events in one order and win the
// append race in the other, so the head can name an event that sorts after one
// that is still unchained; resuming from the head would step over that event
// forever.
func (s *auditStore) sealWatermark(ctx context.Context) (string, error) {
	var watermark string
	err := s.db.NewSelect().
		TableExpr("ee_audit_chain_state").
		ColumnExpr("sealed_event_id").
		Where("scope = ?", auditChainScope).
		Limit(1).
		Scan(ctx, &watermark)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return watermark, nil
}

// advanceSealWatermark records the resume point reached by one reconciliation
// pass. Concurrent passes may occasionally move it backward, which is safe:
// sealing is idempotent and the next pass merely replays already sealed rows.
// Event identities cannot be compared here because IDs from separate
// processes are not globally monotonic.
func (s *auditStore) advanceSealWatermark(ctx context.Context, eventID string) error {
	if err := s.ensureChainState(ctx, s.db); err != nil {
		return err
	}
	_, err := s.db.NewUpdate().
		TableExpr("ee_audit_chain_state").
		Set("sealed_event_id = ?", eventID).
		Set("updated_at = ?", time.Now().UTC()).
		Where("scope = ?", auditChainScope).
		Exec(ctx)
	return err
}

// appendRecord writes one chain record and advances the head only if the head
// is still the one the caller hashed against. A head that moved underneath the
// caller reports [errChainConflict] and rolls the record back.
func (s *auditStore) appendRecord(ctx context.Context, record auditChainRecord, previousIndex int64) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := s.ensureChainState(ctx, tx); err != nil {
			return err
		}

		// The head is claimed before the record is written. Claiming first is
		// what turns a lost race into [errChainConflict] the caller can retry:
		// writing first would instead surface whichever constraint the winner
		// happened to violate, and on PostgreSQL would abort the transaction
		// before the head could be inspected at all.
		result, err := tx.NewUpdate().
			TableExpr("ee_audit_chain_state").
			Set("last_index = ?", record.Index).
			Set("last_hash = ?", record.Hash).
			Set("last_event_id = ?", record.EventID).
			Set("updated_at = ?", record.CreatedAt).
			Where("scope = ?", auditChainScope).
			Where("last_index = ?", previousIndex).
			Exec(ctx)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return errChainConflict
		}

		values := map[string]any{
			"event_id":       record.EventID,
			"scope":          auditChainScope,
			"chain_index":    record.Index,
			"previous_hash":  record.PreviousHash,
			"hash":           record.Hash,
			"signature":      record.Signature,
			"signing_key_id": record.SigningKeyID,
			"retain_until":   record.RetainUntil,
			"created_at":     record.CreatedAt,
		}
		_, err = tx.NewInsert().TableExpr("ee_audit_chain_records").Model(&values).Exec(ctx)
		return err
	})
}

// chainRecords returns every chain record in index order.
func (s *auditStore) chainRecords(ctx context.Context) ([]auditChainRecord, error) {
	var records []auditChainRecord
	err := s.db.NewSelect().
		TableExpr("ee_audit_chain_records").
		ColumnExpr("event_id, chain_index, previous_hash, hash, signature, signing_key_id, retain_until, created_at").
		Where("scope = ?", auditChainScope).
		Order("chain_index ASC").
		Scan(ctx, &records)
	return records, err
}

// chainRecord returns the record protecting one event.
func (s *auditStore) chainRecord(ctx context.Context, eventID string) (auditChainRecord, bool, error) {
	var record auditChainRecord
	err := s.db.NewSelect().
		TableExpr("ee_audit_chain_records").
		ColumnExpr("event_id, chain_index, previous_hash, hash, signature, signing_key_id, retain_until, created_at").
		Where("event_id = ?", eventID).
		Limit(1).
		Scan(ctx, &record)
	if errors.Is(err, sql.ErrNoRows) {
		return auditChainRecord{}, false, nil
	}
	if err != nil {
		return auditChainRecord{}, false, err
	}
	return record, true, nil
}

// applySeal records the signature produced for an already-chained record. The
// hash is never rewritten, so signing cannot alter tamper evidence.
func (s *auditStore) applySeal(ctx context.Context, eventID, signature, keyID string) error {
	_, err := s.db.NewUpdate().
		TableExpr("ee_audit_chain_records").
		Set("signature = ?", signature).
		Set("signing_key_id = ?", keyID).
		Where("event_id = ?", eventID).
		Exec(ctx)
	return err
}

// applyRetention stamps the retention lock a record is held under. The lock is
// policy metadata rather than evidence, so it is deliberately outside the
// hashed payload: changing a retention policy must not invalidate the chain.
func (s *auditStore) applyRetention(ctx context.Context, eventID string, retainUntil *time.Time) error {
	_, err := s.db.NewUpdate().
		TableExpr("ee_audit_chain_records").
		Set("retain_until = ?", retainUntil).
		Where("event_id = ?", eventID).
		Exec(ctx)
	return err
}

// retainSeconds returns the retention lock that applies to an organization,
// falling back to the instance-wide lock stored with a null organization. A
// zero result means no lock is configured.
func (s *auditStore) retainSeconds(ctx context.Context, orgID *int64) (int64, error) {
	query := s.db.NewSelect().
		TableExpr("ee_audit_retention_locks").
		ColumnExpr("retain_seconds").
		Limit(1)
	if orgID == nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ? OR org_id IS NULL", *orgID).
			OrderExpr("CASE WHEN org_id IS NULL THEN 1 ELSE 0 END")
	}

	var seconds int64
	err := query.Scan(ctx, &seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return seconds, nil
}

// exportCursor is the delivery position of one exporter.
//
// Delivered fields are contiguous: every chained event up to DeliveredIndex
// has been accepted by the exporter, so delivery resumes at the next event
// after DeliveredEventID. Pending fields name the oldest event the exporter
// still owes, and LastErrorClass is a bounded classification rather than the
// exporter's own error text.
type exportCursor struct {
	Exporter         string `bun:"exporter"`
	DeliveredIndex   int64  `bun:"delivered_index"`
	DeliveredEventID string `bun:"delivered_event_id"`
	PendingIndex     int64  `bun:"pending_index"`
	PendingEventID   string `bun:"pending_event_id"`
	FailureCount     int64  `bun:"failure_count"`
	LastErrorClass   string `bun:"last_error_class"`
}

func (s *auditStore) exportCursor(ctx context.Context, exporter string) (exportCursor, bool, error) {
	var cursor exportCursor
	err := s.db.NewSelect().
		TableExpr("ee_audit_export_cursors").
		ColumnExpr("exporter, delivered_index, delivered_event_id, pending_index, pending_event_id, failure_count, last_error_class").
		Where("exporter = ?", exporter).
		Limit(1).
		Scan(ctx, &cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return exportCursor{}, false, nil
	}
	if err != nil {
		return exportCursor{}, false, err
	}
	return cursor, true, nil
}

// recordDelivery advances the delivered position by exactly one confirmed
// event and clears the backlog the delivery resolved.
//
// The upsert refuses to move the delivered position backwards: a cursor is a
// statement about what an exporter has actually received, so a late or
// duplicated confirmation for an older event must not rewind it.
func (s *auditStore) recordDelivery(ctx context.Context, exporter, eventID string, index int64, now time.Time) error {
	values := map[string]any{
		"exporter":           exporter,
		"delivered_index":    index,
		"delivered_event_id": eventID,
		"pending_index":      int64(0),
		"pending_event_id":   "",
		"failure_count":      int64(0),
		"last_error_class":   "",
		"updated_at":         now,
	}
	_, err := s.db.NewInsert().
		TableExpr("ee_audit_export_cursors").
		Model(&values).
		On("CONFLICT (exporter) DO UPDATE").
		Set("delivered_index = EXCLUDED.delivered_index").
		Set("delivered_event_id = EXCLUDED.delivered_event_id").
		Set("pending_index = EXCLUDED.pending_index").
		Set("pending_event_id = EXCLUDED.pending_event_id").
		Set("failure_count = EXCLUDED.failure_count").
		Set("last_error_class = EXCLUDED.last_error_class").
		Set("updated_at = EXCLUDED.updated_at").
		Where("ee_audit_export_cursors.delivered_index < EXCLUDED.delivered_index").
		Exec(ctx)
	return err
}

// recordDeliveryFailure remembers the oldest event the exporter still owes and
// counts the attempt. Delivery always resumes from the delivered position, so
// the pending event is whichever event the drain stopped on; it is never
// cleared by a later event succeeding, because no later event is attempted
// until this one is delivered.
//
// class is a bounded failure classification. The exporter's own error text is
// never stored: it is attacker- or vendor-controlled and can carry endpoint
// detail that does not belong in the metadata database.
func (s *auditStore) recordDeliveryFailure(ctx context.Context, exporter, eventID string, index int64, class string, now time.Time) error {
	values := map[string]any{
		"exporter":         exporter,
		"pending_index":    index,
		"pending_event_id": eventID,
		"failure_count":    int64(1),
		"last_error_class": class,
		"updated_at":       now,
	}
	_, err := s.db.NewInsert().
		TableExpr("ee_audit_export_cursors").
		Model(&values).
		On("CONFLICT (exporter) DO UPDATE").
		Set("pending_index = EXCLUDED.pending_index").
		Set("pending_event_id = EXCLUDED.pending_event_id").
		Set("failure_count = ee_audit_export_cursors.failure_count + 1").
		Set("last_error_class = EXCLUDED.last_error_class").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
