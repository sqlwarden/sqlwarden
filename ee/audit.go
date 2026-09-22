package ee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
)

// CapabilityAuditTamperEvidence identifies hash-chained, signed, exportable
// audit records in capability responses.
const CapabilityAuditTamperEvidence = "audit.tamper_evidence"

// Audit pipeline stage names. The order they appear in [auditWriter.pipeline]
// is the contract: a record is durable before it is chained, chained before it
// is locked, locked before it is signed, and signed before it leaves the
// instance.
const (
	stageCoreWrite     = "core-write"
	stageHashChain     = "hash-chain"
	stageRetentionLock = "retention-lock"
	stageSigning       = "signing"
	stageExport        = "siem-export"
)

// defaultAuditRetention is the lock applied when no retention lock row covers
// an event.
const defaultAuditRetention = 365 * 24 * time.Hour

// chainAppendAttempts bounds the retries of an append that lost its race with
// another process appending to the same chain. Retrying re-reads the head and
// re-hashes against it; the bound keeps a contended or wedged chain from
// spinning instead of reporting a degraded stage.
const chainAppendAttempts = 5

// tailBatchSize bounds how many durable records one reconciliation pass reads
// at a time, so catching up after an outage cannot load the whole trail.
const tailBatchSize = 256

// Bounded classifications stored on an export cursor. An exporter's own error
// text is never persisted or logged: it is vendor-controlled and can carry
// endpoint, credential, or payload detail.
const (
	exportFailureCanceled = "canceled"
	exportFailureTimeout  = "timeout"
	exportFailureRejected = "rejected"
)

// Seal is the tamper evidence produced for one audit event.
type Seal struct {
	// Index is the position of the event in the append-only chain, starting
	// at 1.
	Index int64
	// PreviousHash is the chain head the event was hashed against. It is empty
	// for the first event.
	PreviousHash string
	// Hash is the chain hash covering the previous hash, the index, and the
	// canonical event payload.
	Hash string
	// Signature and SigningKeyID are set when a signer is configured.
	Signature    string
	SigningKeyID string
	// RetainUntil is the earliest time the record may be deleted. A nil value
	// means no retention lock applies.
	RetainUntil *time.Time
}

// Signer signs an audit chain hash. Implementations hold the key material;
// the writer only passes the hash it produced.
type Signer interface {
	// Sign returns a detached signature over hash and the identifier of the
	// key that produced it.
	Sign(ctx context.Context, hash string) (signature string, keyID string, err error)
}

// Exporter delivers audit events to an external SIEM. Delivery is explicitly
// best effort with respect to the use case that produced the event: the audit
// record is already durable before Export is called, and an export failure is
// recorded on the exporter's delivery cursor for retry rather than returned to
// the caller.
type Exporter interface {
	// Name identifies the exporter's delivery cursor. It must be stable
	// across restarts.
	Name() string
	Export(ctx context.Context, event audit.Event, seal Seal) error
}

// AuditOptions configures the Enterprise audit decorator.
type AuditOptions struct {
	// Signer is the optional signing hook. A nil signer skips signing without
	// affecting the rest of the pipeline.
	Signer Signer
	// Exporter is the optional SIEM delivery adapter. A nil exporter skips
	// export.
	Exporter Exporter
	// DefaultRetention is the retention lock applied when no retention lock
	// row covers an event. A non-positive value selects one year.
	DefaultRetention time.Duration
}

// auditChainRecord is one row of the tamper-evident chain.
type auditChainRecord struct {
	EventID      string     `bun:"event_id"`
	Index        int64      `bun:"chain_index"`
	PreviousHash string     `bun:"previous_hash"`
	Hash         string     `bun:"hash"`
	Signature    string     `bun:"signature"`
	SigningKeyID string     `bun:"signing_key_id"`
	RetainUntil  *time.Time `bun:"retain_until"`
	CreatedAt    time.Time  `bun:"created_at"`
}

// auditWriter is the Enterprise audit decorator. It adds tamper evidence,
// retention locks, signing, and SIEM export on top of a core writer.
//
// Failure policy, in the order the pipeline runs:
//
//   - Core write failure: the audit intent never became durable, so the error
//     is returned. Recording is mandatory and the failure is not hidden.
//   - Every stage after the core write: the audit record is already durable,
//     so the failure is logged and left for reconciliation, never returned.
//     Returning it would report a committed mutation as failed while the trail
//     itself is intact.
//
// Reconciliation is not a side channel: every stage after the core write
// derives its work from the durable core records themselves. Each write drains
// the records that follow the chain head, so evidence that could not be
// produced during one write is produced during a later one, and a gap that
// survives is exactly what [VerifyAuditChain] reports.
type auditWriter struct {
	core    audit.Writer
	events  audit.Reader
	store   *auditStore
	options AuditOptions
	now     func() time.Time
	logger  *slog.Logger
	tailMu  sync.Mutex
}

func newAuditWriter(core audit.Writer, deps edition.Dependencies, options AuditOptions) *auditWriter {
	deps = deps.Normalize()
	if options.DefaultRetention <= 0 {
		options.DefaultRetention = defaultAuditRetention
	}
	writer := &auditWriter{
		core:    core,
		events:  deps.AuditEvents,
		store:   newAuditStore(deps.DB),
		options: options,
		now:     deps.Now,
		logger:  deps.Logger,
	}
	if writer.events == nil || writer.store == nil {
		deps.Logger.Error("enterprise audit tamper evidence is not configured; audit records will be durable but unchained")
	}
	return writer
}

// auditPhase groups the stages by what they may do on failure.
type auditPhase int

const (
	// phaseDurable establishes durability and is the only phase whose failure
	// reaches the caller.
	phaseDurable auditPhase = iota
	// phaseSeal produces per-record evidence from an already durable record.
	phaseSeal
	// phaseExport delivers an already sealed record off the instance.
	phaseExport
)

// auditStage is one named step of the pipeline. Only a fatal stage aborts the
// write and reports to the caller.
type auditStage struct {
	name  string
	fatal bool
	phase auditPhase
	run   func(ctx context.Context, event audit.Event, seal *Seal) error
}

// pipeline returns the ordered audit stages. The order is the security
// contract of the decorator and is asserted by tests.
func (w *auditWriter) pipeline() []auditStage {
	return []auditStage{
		{name: stageCoreWrite, fatal: true, phase: phaseDurable, run: w.writeCore},
		{name: stageHashChain, fatal: false, phase: phaseSeal, run: w.chain},
		{name: stageRetentionLock, fatal: false, phase: phaseSeal, run: w.lockRetention},
		{name: stageSigning, fatal: false, phase: phaseSeal, run: w.sign},
		{name: stageExport, fatal: false, phase: phaseExport, run: w.export},
	}
}

// Write implements [audit.Writer].
func (w *auditWriter) Write(ctx context.Context, event audit.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}
	// Normalizing before the core write fixes the event identity and time for
	// every stage, so the hash the chain commits to is the hash of the record
	// that was actually stored.
	event = audit.Normalize(event, w.now)

	if err := w.writeCore(ctx, event, nil); err != nil {
		return fmt.Errorf("audit stage %s: %w", stageCoreWrite, err)
	}
	w.settle(ctx)
	return nil
}

// settle runs every stage after the core write against the durable records
// that are not evidenced yet. It never reports an error: by the time it runs
// the audit record is durable and the use case has succeeded.
func (w *auditWriter) settle(ctx context.Context) {
	if w.events == nil || w.store == nil {
		return
	}

	w.tailMu.Lock()
	defer w.tailMu.Unlock()

	if err := w.sealBacklog(ctx); err != nil {
		w.degraded(ctx, err)
	}
	if err := w.exportBacklog(ctx); err != nil {
		w.degraded(ctx, err)
	}
}

func (w *auditWriter) degraded(ctx context.Context, err error) {
	w.logger.WarnContext(ctx, "audit evidence degraded, pending reconciliation", "error", err)
}

// sealBacklog chains, locks, and signs every durable record after the seal
// watermark, oldest first, and stops at the first record it cannot finish: a
// chain that skips a record is no longer a chain.
//
// The watermark only advances over records whose evidence is complete, so a
// stage that failed part way through a record, and a record that was never
// chained at all, are both revisited by the next pass. Sealing is idempotent,
// which is what makes re-reading a settled prefix harmless.
func (w *auditWriter) sealBacklog(ctx context.Context) error {
	watermark, err := w.store.sealWatermark(ctx)
	if err != nil {
		return err
	}

	sealed := watermark
	defer func() {
		if sealed == watermark {
			return
		}
		if err := w.store.advanceSealWatermark(ctx, sealed); err != nil {
			w.degraded(ctx, fmt.Errorf("advance audit seal watermark: %w", err))
		}
	}()

	for {
		events, err := w.events.EventsAfter(ctx, sealed, tailBatchSize)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := w.sealEvent(ctx, event); err != nil {
				return err
			}
			sealed = event.ID
		}
		if len(events) < tailBatchSize {
			return nil
		}
	}
}

func (w *auditWriter) sealEvent(ctx context.Context, event audit.Event) error {
	var seal Seal
	for _, stage := range w.pipeline() {
		if stage.phase != phaseSeal {
			continue
		}
		if err := stage.run(ctx, event, &seal); err != nil {
			return fmt.Errorf("audit stage %s for event %s: %w", stage.name, event.ID, err)
		}
	}
	return nil
}

// exportBacklog delivers every sealed record after the delivered position,
// oldest first, and stops at the first one the exporter refuses. Delivery is
// contiguous by construction: a later event is never attempted before an
// earlier one has been accepted, so a success can neither skip nor clear an
// older backlog.
func (w *auditWriter) exportBacklog(ctx context.Context) error {
	exporter := w.options.Exporter
	if exporter == nil {
		return nil
	}

	cursor, _, err := w.store.exportCursor(ctx, exporter.Name())
	if err != nil {
		return err
	}
	for {
		events, err := w.events.EventsAfter(ctx, cursor.DeliveredEventID, tailBatchSize)
		if err != nil {
			return err
		}
		for _, event := range events {
			record, found, err := w.store.chainRecord(ctx, event.ID)
			if err != nil {
				return err
			}
			if !found {
				// The record is not sealed yet, so it has no seal to export.
				// A later pass picks it up once sealing catches up.
				return nil
			}
			seal := sealFromRecord(record)
			if err := w.export(ctx, event, &seal); err != nil {
				return err
			}
			cursor.DeliveredEventID = event.ID
		}
		if len(events) < tailBatchSize {
			return nil
		}
	}
}

func (w *auditWriter) writeCore(ctx context.Context, event audit.Event, _ *Seal) error {
	return w.core.Write(ctx, event)
}

// chain appends the event to the hash chain. It is idempotent: a record that
// is already chained adopts its stored evidence, so a reconciliation pass that
// overlaps an earlier one cannot append an event twice.
func (w *auditWriter) chain(ctx context.Context, event audit.Event, seal *Seal) error {
	existing, found, err := w.store.chainRecord(ctx, event.ID)
	if err != nil {
		return err
	}
	if found {
		*seal = sealFromRecord(existing)
		return nil
	}

	var lastErr error
	for range chainAppendAttempts {
		head, err := w.store.head(ctx)
		if err != nil {
			return err
		}
		seal.Index = head.Index + 1
		seal.PreviousHash = head.Hash
		seal.Hash = chainHash(head.Hash, seal.Index, event)

		record := auditChainRecord{
			EventID:      event.ID,
			Index:        seal.Index,
			PreviousHash: seal.PreviousHash,
			Hash:         seal.Hash,
			CreatedAt:    w.now().UTC(),
		}
		err = w.store.appendRecord(ctx, record, head.Index)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errChainConflict) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("append refused after %d attempts: %w", chainAppendAttempts, lastErr)
}

func (w *auditWriter) lockRetention(ctx context.Context, event audit.Event, seal *Seal) error {
	seconds, err := w.store.retainSeconds(ctx, event.OrgID)
	if err != nil {
		return err
	}
	retention := w.options.DefaultRetention
	if seconds > 0 {
		retention = time.Duration(seconds) * time.Second
	}
	retainUntil := event.OccurredAt.Add(retention).UTC()
	seal.RetainUntil = &retainUntil
	return w.store.applyRetention(ctx, event.ID, seal.RetainUntil)
}

func (w *auditWriter) sign(ctx context.Context, event audit.Event, seal *Seal) error {
	if w.options.Signer == nil {
		return nil
	}
	if seal.Signature != "" {
		return nil
	}
	signature, keyID, err := w.options.Signer.Sign(ctx, seal.Hash)
	if err != nil {
		return err
	}
	seal.Signature = signature
	seal.SigningKeyID = keyID
	return w.store.applySeal(ctx, event.ID, signature, keyID)
}

// export delivers one sealed record and records the outcome on the cursor. The
// error it returns carries only the bounded failure class, so neither the log
// nor the cursor can end up holding exporter-supplied text.
func (w *auditWriter) export(ctx context.Context, event audit.Event, seal *Seal) error {
	exporter := w.options.Exporter
	if exporter == nil {
		return nil
	}
	if err := exporter.Export(ctx, event, *seal); err != nil {
		class := exportFailureClass(err)
		if cursorErr := w.store.recordDeliveryFailure(ctx, exporter.Name(), event.ID, event.Sequence, class, w.now().UTC()); cursorErr != nil {
			return errors.Join(fmt.Errorf("export event %s: %s", event.ID, class), cursorErr)
		}
		return fmt.Errorf("export event %s: %s", event.ID, class)
	}
	return w.store.recordDelivery(ctx, exporter.Name(), event.ID, event.Sequence, w.now().UTC())
}

// exportFailureClass reduces an exporter error to one of a closed set of
// classes. Everything an exporter can say about a delivery collapses into
// whether the attempt was abandoned, timed out, or refused.
func exportFailureClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return exportFailureCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return exportFailureTimeout
	default:
		return exportFailureRejected
	}
}

func sealFromRecord(record auditChainRecord) Seal {
	return Seal{
		Index:        record.Index,
		PreviousHash: record.PreviousHash,
		Hash:         record.Hash,
		Signature:    record.Signature,
		SigningKeyID: record.SigningKeyID,
		RetainUntil:  record.RetainUntil,
	}
}

// chainHash binds an event to its predecessor. The input is the previous hash,
// the chain index, and the canonical event payload, so neither reordering an
// event nor editing a stored field can reproduce the same hash.
func chainHash(previousHash string, index int64, event audit.Event) string {
	digest := sha256.New()
	digest.Write([]byte(previousHash))
	digest.Write([]byte{0})
	digest.Write([]byte(strconv.FormatInt(index, 10)))
	digest.Write([]byte{0})
	digest.Write(canonicalEvent(event))
	return hex.EncodeToString(digest.Sum(nil))
}

// canonicalEvent serializes an event deterministically: fixed field order,
// sorted metadata keys, and a separator that cannot appear in a field value's
// encoding, so two different events cannot canonicalize to the same bytes.
func canonicalEvent(event audit.Event) []byte {
	var builder strings.Builder
	write := func(field, value string) {
		builder.WriteString(field)
		builder.WriteByte('=')
		builder.WriteString(strconv.Quote(value))
		builder.WriteByte('\n')
	}
	writeID := func(field string, value *int64) {
		if value == nil {
			write(field, "")
			return
		}
		write(field, strconv.FormatInt(*value, 10))
	}

	write("id", event.ID)
	write("occurred_at", event.OccurredAt.UTC().Format(time.RFC3339Nano))
	writeID("org_id", event.OrgID)
	writeID("account_id", event.AccountID)
	write("action", event.Action)
	write("resource", event.Resource)
	write("resource_id", event.ResourceID)
	write("outcome", event.Outcome)

	keys := make([]string, 0, len(event.Metadata))
	for key := range event.Metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		write("metadata."+key, event.Metadata[key])
	}
	return []byte(builder.String())
}

// VerifyAuditChain recomputes the hash chain from the durable core records and
// reports the first piece of evidence that no longer holds. It returns the
// number of verified records.
//
// Verification covers three independent ways evidence can be wrong, because
// each is a different attack: a record whose stored event no longer hashes to
// its chain hash (an edit), a chain whose head does not match its last record
// (a truncated tail), and a durable core event that no chain record covers (an
// event slipped in, or its evidence removed). Records are read through
// [audit.Reader] rather than from a copy kept beside the chain, so editing a
// stored audit row is exactly what the check detects.
func VerifyAuditChain(ctx context.Context, db *database.DB, events audit.Reader) (int, error) {
	store := newAuditStore(db)
	if store == nil {
		return 0, errors.New("enterprise audit storage is not configured")
	}
	records, err := store.chainRecords(ctx)
	if err != nil {
		return 0, err
	}
	recorded, err := events.Events(ctx, 0)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]audit.Event, len(recorded))
	for _, event := range recorded {
		byID[event.ID] = event
	}

	previousHash := ""
	chained := make(map[string]struct{}, len(records))
	for position, record := range records {
		expectedIndex := int64(position + 1)
		if record.Index != expectedIndex {
			return position, fmt.Errorf("audit chain index %d is out of order at position %d", record.Index, expectedIndex)
		}
		if record.PreviousHash != previousHash {
			return position, fmt.Errorf("audit chain record %q does not follow its predecessor", record.EventID)
		}
		event, found := byID[record.EventID]
		if !found {
			return position, fmt.Errorf("audit event %q is missing from durable storage", record.EventID)
		}
		if chainHash(previousHash, record.Index, event) != record.Hash {
			return position, fmt.Errorf("audit event %q does not match its chain hash", record.EventID)
		}
		previousHash = record.Hash
		chained[record.EventID] = struct{}{}
	}

	head, err := store.head(ctx)
	if err != nil {
		return len(records), err
	}
	if err := verifyChainHead(head, records, previousHash); err != nil {
		return len(records), err
	}

	for _, event := range recorded {
		if _, found := chained[event.ID]; !found {
			return len(records), fmt.Errorf("audit event %q is not covered by the chain", event.ID)
		}
	}
	return len(records), nil
}

// verifyChainHead reports a chain whose recorded head does not describe its
// last record. A truncated tail leaves the head ahead of the records that
// remain, which no per-record check can see.
func verifyChainHead(head chainHead, records []auditChainRecord, lastHash string) error {
	if head.Index != int64(len(records)) {
		return fmt.Errorf("audit chain head is at index %d but %d records remain", head.Index, len(records))
	}
	if head.Hash != lastHash {
		return errors.New("audit chain head hash does not match the last chain record")
	}
	lastEventID := ""
	if len(records) > 0 {
		lastEventID = records[len(records)-1].EventID
	}
	if head.EventID != lastEventID {
		return fmt.Errorf("audit chain head names event %q but the last chain record is %q", head.EventID, lastEventID)
	}
	return nil
}

var _ audit.Writer = (*auditWriter)(nil)
