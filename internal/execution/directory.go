package execution

import (
	"context"
	"sync"
	"time"
)

// DirectoryRecord is the complete routing lease for a live session. Presence
// summaries are advisory routing metadata; the owning runtime remains the
// authority for transaction and cursor state.
type DirectoryRecord struct {
	Handle               SessionHandle `json:"session_handle"`
	Scope                Scope         `json:"scope"`
	OwnerRuntimeID       string        `json:"owner_runtime_id"`
	RoutingAddress       string        `json:"routing_address"`
	CreatedAt            time.Time     `json:"created_at"`
	LastSeenAt           time.Time     `json:"last_seen_at"`
	LeaseExpiresAt       time.Time     `json:"lease_expires_at"`
	HasOpenTransaction   bool          `json:"has_open_transaction"`
	HasOpenCursors       bool          `json:"has_open_cursors"`
	ProtocolVersion      uint32        `json:"protocol_version"`
	RevocationGeneration uint64        `json:"revocation_generation"`
}

// SessionDirectory locates the runtime that owns an opaque session. Its shape
// is deliberately shared by the in-memory and future Redis adapters.
type SessionDirectory interface {
	Put(context.Context, DirectoryRecord) error
	Get(context.Context, SessionHandle) (DirectoryRecord, bool, error)
	Renew(context.Context, SessionHandle, time.Time) error
	Delete(context.Context, SessionHandle) error
}

// MemorySessionDirectory is the single-process directory adapter.
type MemorySessionDirectory struct {
	mu      sync.RWMutex
	records map[SessionHandle]DirectoryRecord
	now     func() time.Time
}

// NewMemorySessionDirectory returns an empty process-local session directory.
func NewMemorySessionDirectory() *MemorySessionDirectory {
	return &MemorySessionDirectory{records: make(map[SessionHandle]DirectoryRecord), now: time.Now}
}

// Put creates or replaces a session routing lease.
func (d *MemorySessionDirectory) Put(_ context.Context, record DirectoryRecord) error {
	d.mu.Lock()
	d.records[record.Handle] = record
	d.mu.Unlock()
	return nil
}

// Get returns an unexpired routing lease, lazily removing expired records.
func (d *MemorySessionDirectory) Get(_ context.Context, handle SessionHandle) (DirectoryRecord, bool, error) {
	d.mu.RLock()
	record, ok := d.records[handle]
	d.mu.RUnlock()
	if !ok {
		return DirectoryRecord{}, false, nil
	}
	if !record.LeaseExpiresAt.IsZero() && !d.now().Before(record.LeaseExpiresAt) {
		d.mu.Lock()
		if current, exists := d.records[handle]; exists && current.LeaseExpiresAt.Equal(record.LeaseExpiresAt) {
			delete(d.records, handle)
		}
		d.mu.Unlock()
		return DirectoryRecord{}, false, nil
	}
	return record, true, nil
}

// Renew advances an existing routing lease and its last-seen timestamp.
func (d *MemorySessionDirectory) Renew(_ context.Context, handle SessionHandle, expiresAt time.Time) error {
	d.mu.Lock()
	if record, ok := d.records[handle]; ok {
		record.LastSeenAt = d.now()
		record.LeaseExpiresAt = expiresAt
		d.records[handle] = record
	}
	d.mu.Unlock()
	return nil
}

// Delete removes a routing lease. It is idempotent.
func (d *MemorySessionDirectory) Delete(_ context.Context, handle SessionHandle) error {
	d.mu.Lock()
	delete(d.records, handle)
	d.mu.Unlock()
	return nil
}

var _ SessionDirectory = (*MemorySessionDirectory)(nil)
