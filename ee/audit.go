//go:build enterprise

package ee

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/edition"
	"github.com/uptrace/bun"
)

const auditScope = "instance"
const auditBatchSize = 256
const chainAppendAttempts = 5
const defaultRetention = 365 * 24 * time.Hour

var errChainConflict = errors.New("audit chain head changed during append")

type Seal struct {
	Index        int64
	PreviousHash string
	Hash         string
	Signature    string
	SigningKeyID string
	RetainUntil  *time.Time
}

type Signer interface {
	Sign(context.Context, string) (signature, keyID string, err error)
}
type Exporter interface {
	Name() string
	Export(context.Context, audit.Event, Seal) error
}

type AuditOptions struct {
	Signer           Signer
	Exporter         Exporter
	DefaultRetention time.Duration
}

type AuditWriter struct {
	core    audit.Writer
	events  audit.Reader
	db      bun.IDB
	logger  *slog.Logger
	now     func() time.Time
	options AuditOptions
	mu      sync.Mutex
}

func NewAuditWriter(core audit.Writer, deps edition.Dependencies, options AuditOptions) *AuditWriter {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if options.DefaultRetention <= 0 {
		options.DefaultRetention = defaultRetention
	}
	return &AuditWriter{core: core, events: deps.AuditEvents, db: deps.SQL, logger: deps.Logger, now: deps.Now, options: options}
}

func (w *AuditWriter) Write(ctx context.Context, event audit.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	event = audit.Normalize(event, w.now)
	if err := w.core.Write(ctx, event); err != nil {
		return fmt.Errorf("audit stage core-write: %w", err)
	}
	if err := w.settle(ctx); err != nil {
		w.logger.WarnContext(ctx, "audit evidence degraded, pending reconciliation", "error", err)
	}
	return nil
}

func (w *AuditWriter) settle(ctx context.Context) error {
	if w.events == nil || w.db == nil {
		return errors.New("enterprise audit dependencies are unavailable")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureState(ctx); err != nil {
		return err
	}
	watermark, err := w.sealWatermark(ctx)
	if err != nil {
		return err
	}
	for {
		events, err := w.events.EventsAfter(ctx, watermark, auditBatchSize)
		if err != nil {
			return err
		}
		for _, event := range events {
			if _, err := w.seal(ctx, event); err != nil {
				return err
			}
			watermark = event.ID
			if err := w.advanceSealWatermark(ctx, watermark); err != nil {
				return err
			}
		}
		if len(events) < auditBatchSize {
			break
		}
	}
	return w.export(ctx)
}

type chainHead struct {
	Index   int64  `bun:"last_index"`
	Hash    string `bun:"last_hash"`
	EventID string `bun:"last_event_id"`
}

func (w *AuditWriter) ensureState(ctx context.Context) error {
	_, err := w.db.NewInsert().TableExpr("ee_audit_chain_state").Model(&map[string]any{
		"scope": auditScope, "last_index": int64(0), "last_hash": "", "last_event_id": "", "sealed_event_id": "", "updated_at": w.now().UTC(),
	}).On("CONFLICT (scope) DO NOTHING").Exec(ctx)
	return err
}

func (w *AuditWriter) sealWatermark(ctx context.Context) (string, error) {
	var watermark string
	err := w.db.NewSelect().TableExpr("ee_audit_chain_state").ColumnExpr("sealed_event_id").Where("scope = ?", auditScope).Scan(ctx, &watermark)
	return watermark, err
}

func (w *AuditWriter) advanceSealWatermark(ctx context.Context, eventID string) error {
	_, err := w.db.NewUpdate().TableExpr("ee_audit_chain_state").
		Set("sealed_event_id = ?", eventID).Set("updated_at = ?", w.now().UTC()).
		Where("scope = ?", auditScope).Exec(ctx)
	return err
}

func (w *AuditWriter) head(ctx context.Context) (chainHead, error) {
	var head chainHead
	err := w.db.NewSelect().TableExpr("ee_audit_chain_state").ColumnExpr("last_index, last_hash, last_event_id").Where("scope = ?", auditScope).Scan(ctx, &head)
	return head, err
}

func canonical(event audit.Event, index int64, previous string) ([]byte, error) {
	event.Sequence = 0
	return json.Marshal(struct {
		Previous string      `json:"previous"`
		Index    int64       `json:"index"`
		Event    audit.Event `json:"event"`
	}{previous, index, event})
}

func (w *AuditWriter) seal(ctx context.Context, event audit.Event) (Seal, error) {
	if existing, err := w.record(ctx, event.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Seal{}, err
	}
	retention, err := w.retention(ctx, event.OrgID)
	if err != nil {
		return Seal{}, err
	}
	var lastErr error
	for range chainAppendAttempts {
		head, err := w.head(ctx)
		if err != nil {
			return Seal{}, err
		}
		payload, err := canonical(event, head.Index+1, head.Hash)
		if err != nil {
			return Seal{}, err
		}
		digest := sha256.Sum256(payload)
		seal := Seal{Index: head.Index + 1, PreviousHash: head.Hash, Hash: hex.EncodeToString(digest[:])}
		retainUntil := event.OccurredAt.Add(retention).UTC()
		seal.RetainUntil = &retainUntil
		if w.options.Signer != nil {
			seal.Signature, seal.SigningKeyID, err = w.options.Signer.Sign(ctx, seal.Hash)
			if err != nil {
				return Seal{}, fmt.Errorf("sign audit event %s: %w", event.ID, err)
			}
		}
		err = w.appendRecord(ctx, head, event.ID, seal)
		if err == nil {
			return seal, nil
		}
		if !errors.Is(err, errChainConflict) {
			if existing, recordErr := w.record(ctx, event.ID); recordErr == nil {
				return existing, nil
			}
			return Seal{}, err
		}
		lastErr = err
	}
	return Seal{}, fmt.Errorf("append refused after %d attempts: %w", chainAppendAttempts, lastErr)
}

func (w *AuditWriter) appendRecord(ctx context.Context, head chainHead, eventID string, seal Seal) error {
	return w.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().TableExpr("ee_audit_chain_state").
			Set("last_index = ?", seal.Index).Set("last_hash = ?", seal.Hash).
			Set("last_event_id = ?", eventID).Set("updated_at = ?", w.now().UTC()).
			Where("scope = ? AND last_index = ?", auditScope, head.Index).Exec(ctx)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return errChainConflict
		}
		_, err = tx.NewInsert().TableExpr("ee_audit_chain_records").Model(&map[string]any{
			"event_id": eventID, "scope": auditScope, "chain_index": seal.Index,
			"previous_hash": seal.PreviousHash, "hash": seal.Hash, "signature": seal.Signature,
			"signing_key_id": seal.SigningKeyID, "retain_until": seal.RetainUntil, "created_at": w.now().UTC(),
		}).Exec(ctx)
		return err
	})
}

func (w *AuditWriter) retention(ctx context.Context, orgID *int64) (time.Duration, error) {
	query := w.db.NewSelect().TableExpr("ee_audit_retention_locks").ColumnExpr("retain_seconds").Limit(1)
	if orgID == nil {
		query = query.Where("org_id IS NULL")
	} else {
		query = query.Where("org_id = ? OR org_id IS NULL", *orgID).
			OrderExpr("CASE WHEN org_id IS NULL THEN 1 ELSE 0 END")
	}
	var seconds int64
	err := query.Scan(ctx, &seconds)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return w.options.DefaultRetention, nil
		}
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

func (w *AuditWriter) export(ctx context.Context) error {
	if w.options.Exporter == nil {
		return nil
	}
	name := w.options.Exporter.Name()
	if name == "" {
		return errors.New("audit exporter name is required")
	}
	_, err := w.db.NewInsert().TableExpr("ee_audit_export_cursors").Model(&map[string]any{
		"exporter": name, "delivered_event_id": "", "failure_count": int64(0), "last_error_class": "", "updated_at": w.now().UTC(),
	}).On("CONFLICT (exporter) DO NOTHING").Exec(ctx)
	if err != nil {
		return err
	}
	var cursor string
	if err := w.db.NewSelect().TableExpr("ee_audit_export_cursors").ColumnExpr("delivered_event_id").Where("exporter = ?", name).Scan(ctx, &cursor); err != nil {
		return err
	}
	for {
		events, err := w.events.EventsAfter(ctx, cursor, auditBatchSize)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := w.exportEvent(ctx, name, event); err != nil {
				return err
			}
			cursor = event.ID
		}
		if len(events) < auditBatchSize {
			return nil
		}
	}
}

func (w *AuditWriter) exportEvent(ctx context.Context, exporter string, event audit.Event) error {
	seal, err := w.record(ctx, event.ID)
	if err != nil {
		return err
	}
	if exportErr := w.options.Exporter.Export(ctx, event, seal); exportErr != nil {
		_, err := w.db.NewUpdate().TableExpr("ee_audit_export_cursors").
			Set("failure_count = failure_count + 1").Set("last_error_class = ?", exportFailureClass(exportErr)).
			Set("updated_at = ?", w.now().UTC()).Where("exporter = ?", exporter).Exec(ctx)
		return errors.Join(fmt.Errorf("export audit event %s: %w", event.ID, exportErr), err)
	}
	_, err = w.db.NewUpdate().TableExpr("ee_audit_export_cursors").
		Set("delivered_event_id = ?", event.ID).Set("failure_count = 0").Set("last_error_class = ''").
		Set("updated_at = ?", w.now().UTC()).Where("exporter = ?", exporter).Exec(ctx)
	return err
}

// exportFailureClass maps exporter errors to a bounded set so that cursor rows
// never store exporter-controlled text.
func exportFailureClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "rejected"
	}
}

func (w *AuditWriter) record(ctx context.Context, eventID string) (Seal, error) {
	var record struct {
		Index        int64      `bun:"chain_index"`
		PreviousHash string     `bun:"previous_hash"`
		Hash         string     `bun:"hash"`
		Signature    string     `bun:"signature"`
		SigningKeyID string     `bun:"signing_key_id"`
		RetainUntil  *time.Time `bun:"retain_until"`
	}
	err := w.db.NewSelect().TableExpr("ee_audit_chain_records").ColumnExpr("chain_index, previous_hash, hash, signature, signing_key_id, retain_until").Where("event_id = ?", eventID).Scan(ctx, &record)
	return Seal{Index: record.Index, PreviousHash: record.PreviousHash, Hash: record.Hash, Signature: record.Signature, SigningKeyID: record.SigningKeyID, RetainUntil: record.RetainUntil}, err
}

func VerifyAuditChain(ctx context.Context, events audit.Reader, db bun.IDB) error {
	durable, err := events.Events(ctx, 0)
	if err != nil {
		return err
	}
	byID := make(map[string]audit.Event, len(durable))
	for _, event := range durable {
		byID[event.ID] = event
	}
	var records []struct {
		EventID      string `bun:"event_id"`
		Index        int64  `bun:"chain_index"`
		PreviousHash string `bun:"previous_hash"`
		Hash         string `bun:"hash"`
	}
	if err := db.NewSelect().TableExpr("ee_audit_chain_records").
		ColumnExpr("event_id, chain_index, previous_hash, hash").
		Where("scope = ?", auditScope).Order("chain_index ASC").Scan(ctx, &records); err != nil {
		return err
	}
	previous := ""
	covered := make(map[string]struct{}, len(records))
	for position, record := range records {
		event, found := byID[record.EventID]
		if !found {
			return fmt.Errorf("audit event %s is missing from durable storage", record.EventID)
		}
		expectedIndex := int64(position + 1)
		payload, err := canonical(event, expectedIndex, previous)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		if record.Index != expectedIndex || record.PreviousHash != previous || record.Hash != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("audit chain verification failed at event %s", record.EventID)
		}
		previous = record.Hash
		covered[record.EventID] = struct{}{}
	}
	head, err := (&AuditWriter{db: db}).head(ctx)
	if err != nil {
		return err
	}
	lastEventID := ""
	if len(records) > 0 {
		lastEventID = records[len(records)-1].EventID
	}
	if head.Index != int64(len(records)) || head.Hash != previous || head.EventID != lastEventID {
		return errors.New("audit chain head does not match its records")
	}
	for _, event := range durable {
		if _, found := covered[event.ID]; !found {
			return fmt.Errorf("audit event %s is not covered by the chain", event.ID)
		}
	}
	return nil
}

var _ audit.Writer = (*AuditWriter)(nil)
