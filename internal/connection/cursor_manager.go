package connection

import (
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// QueryCursorCreateParams describes the runtime ownership metadata for a live
// query cursor. Authorization is still handled by the caller; the manager only
// owns cursor lifecycle and cleanup.
type QueryCursorCreateParams struct {
	ParentSession *Session
	Cursor        *QueryCursorHandle
}

// QueryCursorRecord tracks one live query cursor. The parent session is the
// authority for account, org, workspace, and connection scope.
type QueryCursorRecord struct {
	ID              string
	ParentSessionID string
	ParentSession   *Session
	CursorID        string
	Cursor          *QueryCursorHandle
	CreatedAt       time.Time
	LastUsedAt      time.Time
	Closed          bool
	Exhausted       bool
	mu              sync.Mutex
}

// QueryCursorManager owns in-memory query cursor records, idle expiry, and
// cleanup of the underlying session cursor handles.
type QueryCursorManager struct {
	mu          sync.RWMutex
	cursors     map[string]*QueryCursorRecord
	idleTimeout time.Duration
	stop        chan struct{}
	stopped     chan struct{}
	closeOnce   sync.Once
}

// NewQueryCursorManager creates a cursor manager and starts its idle reaper.
func NewQueryCursorManager(idleTimeout time.Duration) *QueryCursorManager {
	m := &QueryCursorManager{
		cursors:     make(map[string]*QueryCursorRecord),
		idleTimeout: idleTimeout,
		stop:        make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	go m.reap()
	return m
}

// Create registers a live query cursor and returns its runtime record.
func (m *QueryCursorManager) Create(params QueryCursorCreateParams) *QueryCursorRecord {
	now := time.Now()
	qc := &QueryCursorRecord{
		ID:              ulid.Make().String(),
		ParentSessionID: queryCursorParentSessionID(params.ParentSession),
		ParentSession:   params.ParentSession,
		CursorID:        params.Cursor.ID,
		Cursor:          params.Cursor,
		CreatedAt:       now,
		LastUsedAt:      now,
	}

	m.mu.Lock()
	m.cursors[qc.ID] = qc
	m.mu.Unlock()
	return qc
}

func queryCursorParentSessionID(session *Session) string {
	if session == nil {
		return ""
	}
	return session.ID
}

// Get returns a cursor record by id without changing its idle timestamp.
func (m *QueryCursorManager) Get(id string) (*QueryCursorRecord, bool) {
	m.mu.RLock()
	qc, ok := m.cursors[id]
	m.mu.RUnlock()
	return qc, ok
}

// Remove unregisters and closes a cursor record.
func (m *QueryCursorManager) Remove(id string) bool {
	qc, ok := m.take(id)
	if !ok {
		return false
	}
	return qc.close()
}

// Close stops the idle reaper and closes all remaining cursor records.
func (m *QueryCursorManager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
	})
	<-m.stopped

	m.mu.Lock()
	cursors := make([]*QueryCursorRecord, 0, len(m.cursors))
	for id, qc := range m.cursors {
		cursors = append(cursors, qc)
		delete(m.cursors, id)
	}
	m.mu.Unlock()

	for _, qc := range cursors {
		qc.close()
	}
}

func (m *QueryCursorManager) take(id string) (*QueryCursorRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	qc, ok := m.cursors[id]
	if ok {
		delete(m.cursors, id)
	}
	return qc, ok
}

func (m *QueryCursorManager) reap() {
	defer close(m.stopped)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.ReapIdle(now)
		}
	}
}

// ReapIdle closes and removes cursors whose idle timeout has elapsed.
func (m *QueryCursorManager) ReapIdle(now time.Time) int {
	m.mu.Lock()
	expired := make([]*QueryCursorRecord, 0)
	for id, qc := range m.cursors {
		qc.mu.Lock()
		expiredCursor := qc.Closed || now.Sub(qc.LastUsedAt) > m.idleTimeout
		qc.mu.Unlock()
		if expiredCursor {
			expired = append(expired, qc)
			delete(m.cursors, id)
		}
	}
	m.mu.Unlock()

	for _, qc := range expired {
		qc.close()
	}
	return len(expired)
}

// Touch refreshes the cursor idle timestamp. It returns false if the cursor was
// already closed.
func (qc *QueryCursorRecord) Touch() bool {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	if qc.Closed {
		return false
	}
	qc.LastUsedAt = time.Now()
	return true
}

// MarkExhausted records that the cursor reached the end of its result stream.
func (qc *QueryCursorRecord) MarkExhausted() {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	qc.Exhausted = true
}

// State returns the current terminal flags for logging and diagnostics.
func (qc *QueryCursorRecord) State() (closed bool, exhausted bool) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	return qc.Closed, qc.Exhausted
}

func (qc *QueryCursorRecord) close() bool {
	qc.mu.Lock()
	if qc.Closed {
		qc.mu.Unlock()
		return false
	}
	qc.Closed = true
	qc.mu.Unlock()

	if qc.ParentSession != nil {
		_ = qc.ParentSession.CloseCursor(qc.CursorID)
	} else if qc.Cursor != nil {
		_ = qc.Cursor.Close()
	}
	return true
}
