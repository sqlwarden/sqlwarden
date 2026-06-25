package connection

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/pkg/result"
)

var ErrQueryCursorsUnsupported = errors.New("driver does not support query cursors")

// entropySource is a package-level entropy source for ULID generation.
var (
	entropyMu     sync.Mutex
	entropySource = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// newULID generates a new ULID string in a thread-safe manner.
func newULID() string {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropySource).String()
}

// Session is an open live connection to a target database.
type Session struct {
	ID           string // ULID
	AccountID    string
	ConnectionID string
	OrgID        string
	WorkspaceID  string
	Conn         dbengine.Driver // open connection
	mu           sync.Mutex      // serializes Query/Execute on this session
	cursors      map[string]*QueryCursorHandle
	lastUsed     time.Time
}

type QueryCursorHandle struct {
	ID     string
	Cursor cursor.QueryCursor
	mu     sync.Mutex
}

type SessionMetadata struct {
	OrgID       string
	WorkspaceID string
}

// Query executes a query on the session, serialized via the session mutex.
func (s *Session) Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	return s.Conn.Query(ctx, sql, args...)
}

// Execute executes a statement on the session, serialized via the session mutex.
func (s *Session) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	return s.Conn.Execute(ctx, sql, args...)
}

func (s *Session) StartQueryCursor(ctx context.Context, sql string, args ...any) (*QueryCursorHandle, error) {
	cursorDriver, ok := s.Conn.(cursor.QueryCursorDriver)
	if !ok {
		return nil, ErrQueryCursorsUnsupported
	}

	cursor, err := cursorDriver.StartQuery(ctx, cursor.QueryRequest{SQL: sql, Args: args})
	if err != nil {
		return nil, err
	}

	handle := &QueryCursorHandle{ID: newULID(), Cursor: cursor}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursors == nil {
		s.cursors = make(map[string]*QueryCursorHandle)
	}
	s.cursors[handle.ID] = handle
	s.lastUsed = time.Now()

	return handle, nil
}

func (s *Session) CloseCursor(cursorID string) error {
	s.mu.Lock()
	handle, ok := s.cursors[cursorID]
	if ok {
		delete(s.cursors, cursorID)
	}
	s.lastUsed = time.Now()
	s.mu.Unlock()

	if !ok {
		return nil
	}
	return handle.Close()
}

func (s *Session) CloseAllCursors() {
	s.mu.Lock()
	handles := make([]*QueryCursorHandle, 0, len(s.cursors))
	for id, handle := range s.cursors {
		handles = append(handles, handle)
		delete(s.cursors, id)
	}
	s.mu.Unlock()

	for _, handle := range handles {
		_ = handle.Close()
	}
}

func (h *QueryCursorHandle) Columns() []result.Column {
	return h.Cursor.Columns()
}

func (h *QueryCursorHandle) Fetch(ctx context.Context, opts cursor.ScanOptions) (*result.ResultSet, cursor.QueryCursorState, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Cursor.Fetch(ctx, opts)
}

func (h *QueryCursorHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Cursor.Close()
}

// Manager maintains in-memory live sessions with TTL reaping.
type Manager struct {
	mu          sync.RWMutex
	byKey       map[string]*Session // key: "accountID:connID"
	byID        map[string]*Session // key: session ULID
	idleTimeout time.Duration
	stop        chan struct{}
	stopped     chan struct{}
	closeOnce   sync.Once
}

// New creates a new Manager with the given idle timeout and starts the background reaper.
func New(idleTimeout time.Duration) *Manager {
	m := &Manager{
		byKey:       make(map[string]*Session),
		byID:        make(map[string]*Session),
		idleTimeout: idleTimeout,
		stop:        make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	go m.reap()
	return m
}

// GetOrCreate returns the existing session for (accountID, connID) or creates one using open().
// Returns: (session, created, error) where created=true means a new session was opened.
func (m *Manager) GetOrCreate(accountID, connID string, open func() (dbengine.Driver, error)) (*Session, bool, error) {
	return m.GetOrCreateWithMetadata(accountID, connID, SessionMetadata{}, open)
}

// GetOrCreateWithMetadata returns an existing session or creates one with
// resource metadata used for workspace-scoped admin visibility and revocation.
func (m *Manager) GetOrCreateWithMetadata(accountID, connID string, metadata SessionMetadata, open func() (dbengine.Driver, error)) (*Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, connID)
	if sess, ok := m.byKey[key]; ok {
		sess.lastUsed = time.Now()
		if metadata.OrgID != "" {
			sess.OrgID = metadata.OrgID
		}
		if metadata.WorkspaceID != "" {
			sess.WorkspaceID = metadata.WorkspaceID
		}
		return sess, false, nil
	}

	d, err := open()
	if err != nil {
		return nil, false, err
	}

	sess := &Session{
		ID:           newULID(),
		AccountID:    accountID,
		ConnectionID: connID,
		OrgID:        metadata.OrgID,
		WorkspaceID:  metadata.WorkspaceID,
		Conn:         d,
		lastUsed:     time.Now(),
	}

	m.byKey[key] = sess
	m.byID[sess.ID] = sess

	return sess, true, nil
}

// SessionRef is a lightweight summary of an active session returned by AllForAccount.
type SessionRef struct {
	SessionID    string
	AccountID    string
	ConnectionID string
	OrgID        string
	WorkspaceID  string
	LastUsedAt   time.Time
}

// AllForAccount returns a SessionRef for every active session owned by accountID.
// It does not update lastUsed; use Get to both fetch and refresh a session.
func (m *Manager) AllForAccount(accountID string) []SessionRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var refs []SessionRef
	for _, sess := range m.byID {
		if sess.AccountID == accountID {
			refs = append(refs, SessionRef{
				SessionID:    sess.ID,
				AccountID:    sess.AccountID,
				ConnectionID: sess.ConnectionID,
				OrgID:        sess.OrgID,
				WorkspaceID:  sess.WorkspaceID,
				LastUsedAt:   sess.lastUsed,
			})
		}
	}
	return refs
}

// AllForWorkspace returns active sessions known to belong to workspaceID.
func (m *Manager) AllForWorkspace(workspaceID string) []SessionRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var refs []SessionRef
	for _, sess := range m.byID {
		if sess.WorkspaceID == workspaceID {
			refs = append(refs, SessionRef{
				SessionID:    sess.ID,
				AccountID:    sess.AccountID,
				ConnectionID: sess.ConnectionID,
				OrgID:        sess.OrgID,
				WorkspaceID:  sess.WorkspaceID,
				LastUsedAt:   sess.lastUsed,
			})
		}
	}
	return refs
}

// Get fetches a session by its ID. Returns (session, true) if found, (nil, false) otherwise.
func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.byID[sessionID]
	if !ok {
		return nil, false
	}
	sess.lastUsed = time.Now()
	return sess, true
}

// Remove closes and removes a session by ID.
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.byID[sessionID]
	if !ok {
		return
	}

	sess.close()
	key := sess.AccountID + ":" + sess.ConnectionID
	delete(m.byKey, key)
	delete(m.byID, sessionID)
}

// CountForConnection returns the number of live sessions for the given connection ID.
func (m *Manager) CountForConnection(connID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, sess := range m.byID {
		if sess.ConnectionID == connID {
			count++
		}
	}
	return count
}

// RemoveForConnection closes and removes all live sessions for the given connection ID.
// Returns the number of removed sessions.
func (m *Manager) RemoveForConnection(connID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for id, sess := range m.byID {
		if sess.ConnectionID != connID {
			continue
		}
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
		removed++
	}
	return removed
}

// RemoveForAccount closes and removes all live sessions owned by accountID.
func (m *Manager) RemoveForAccount(accountID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for id, sess := range m.byID {
		if sess.AccountID != accountID {
			continue
		}
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
		removed++
	}
	return removed
}

// RemoveForWorkspaceAccount closes and removes all live sessions for an account
// inside one workspace.
func (m *Manager) RemoveForWorkspaceAccount(workspaceID, accountID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for id, sess := range m.byID {
		if sess.WorkspaceID != workspaceID || sess.AccountID != accountID {
			continue
		}
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
		removed++
	}
	return removed
}

// RemoveForOrgAccount closes and removes all live sessions for an account in
// an organization.
func (m *Manager) RemoveForOrgAccount(orgID, accountID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	removed := 0
	for id, sess := range m.byID {
		if sess.OrgID != orgID || sess.AccountID != accountID {
			continue
		}
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
		removed++
	}
	return removed
}

// Close closes all sessions and stops the reaper goroutine. Safe to call multiple times.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
	})
	<-m.stopped

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, sess := range m.byID {
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
	}
}

func (m *Manager) reap() {
	defer close(m.stopped)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.reapIdle()
		}
	}
}

func (m *Manager) reapIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, sess := range m.byID {
		if now.Sub(sess.lastUsed) > m.idleTimeout {
			sess.close()
			key := sess.AccountID + ":" + sess.ConnectionID
			delete(m.byKey, key)
			delete(m.byID, id)
		}
	}
}

func (s *Session) close() {
	s.CloseAllCursors()
	_ = s.Conn.Close()
}
