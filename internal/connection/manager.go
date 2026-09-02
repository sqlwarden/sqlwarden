package connection

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/pkg/result"
)

var ErrQueryCursorsUnsupported = errors.New("driver does not support query cursors")

// TxMode is a Session's transaction mode: statements commit immediately
// (auto) or accumulate in an open transaction until explicitly committed or
// rolled back (manual).
type TxMode string

const (
	TxModeAuto   TxMode = "auto"
	TxModeManual TxMode = "manual"
)

// ErrTransactionOpen is returned by SetTransactionMode when switching from
// manual to auto while a transaction is still open; the caller must commit
// or roll back first.
var ErrTransactionOpen = errors.New("connection: cannot switch to auto-commit while a transaction is open")

// TransactionStatus is a read-only snapshot of a Session's transaction state.
type TransactionStatus struct {
	Mode              TxMode
	Open              bool
	PendingStatements int
	// Statements is a short human-readable label per statement run since the
	// last commit/rollback, in execution order. In-memory only — never
	// persisted or logged, since it may echo user-authored SQL.
	Statements []string
}

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
	ID                string // ULID
	AccountID         string
	ConnectionID      string
	OrgID             string
	WorkspaceID       string
	Conn              engine.Driver // open connection
	teardown          func()        // released after Conn.Close(); nil when nothing to tear down
	tunnelHealth      func() *bool  // SSH tunnel health probe; nil when the session has no tunnel
	mu                sync.Mutex    // serializes Query/Execute on this session
	cursors           map[string]*QueryCursorHandle
	lastUsed          time.Time
	txMode            TxMode
	pendingStatements []string
	savepointSeq      int
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

// runInTransaction lazily begins a transaction on the first statement after
// a switch to manual mode (or after the last commit/rollback), wraps the
// statement in a savepoint when the driver supports one, and records
// description in pendingStatements. Must be called with s.mu held.
//
// description is a short human-readable label for the statement (the SQL
// text itself, or a DDL request's Summary()), shown to the user as the list
// of statements pending commit/rollback. It is kept in memory only.
//
// In manual mode every cursor still open on this session is closed first: an
// unexhausted cursor pins the transaction's single physical connection, so a
// following statement on that same transaction would contend with it or fail.
// Callers must expect an open cursor to die when another statement runs in
// manual mode, the same way cursors die on commit/rollback.
func (s *Session) runInTransaction(ctx context.Context, description string, statement func() error) error {
	if s.txMode != TxModeManual {
		return statement()
	}
	controller, ok := s.Conn.(transaction.Controller)
	if !ok {
		// Driver has no transaction support at all; manual mode behaves like auto.
		return statement()
	}
	s.closeCursorsLocked()
	if !controller.InTransaction() {
		// A manual-mode transaction is meant to outlive the request that opens
		// it — later statements arrive on separate HTTP requests until the
		// user commits or rolls back. database/sql ties a BeginTx-created
		// transaction's lifetime to the context passed here, silently rolling
		// it back once that context is done; detach it from the triggering
		// request's cancellation the same way query cursors already do (see
		// queryCursorLifetimeContext in internal/web).
		if err := controller.BeginTx(context.WithoutCancel(ctx)); err != nil {
			return err
		}
	}
	savepointController, hasSavepoints := s.Conn.(transaction.SavepointController)
	if !hasSavepoints {
		err := statement()
		s.pendingStatements = append(s.pendingStatements, description)
		if err != nil {
			_ = controller.Rollback(ctx)
			s.pendingStatements = nil
		}
		return err
	}
	s.savepointSeq++
	name := transaction.NewSavepointName(s.savepointSeq)
	if err := savepointController.Savepoint(ctx, name); err != nil {
		return err
	}
	err := statement()
	s.pendingStatements = append(s.pendingStatements, description)
	if err != nil {
		if rbErr := savepointController.RollbackToSavepoint(ctx, name); rbErr != nil {
			// The savepoint recovery itself failed — typically because the
			// connection died underneath it — so there is no partial state
			// left worth preserving. Fully discard the transaction client-side
			// the same way Commit/Rollback already do on driver error, or the
			// session would keep reporting Open=true with no working recovery
			// path: neither another savepoint nor a plain commit/rollback can
			// succeed against a connection that's already gone.
			_ = controller.Rollback(ctx)
			s.pendingStatements = nil
		}
	}
	return err
}

// SetTransactionMode switches between auto-commit and manual-commit.
// Switching to manual always succeeds. Switching to auto while a
// transaction is open returns ErrTransactionOpen; the caller must commit
// or roll back first.
func (s *Session) SetTransactionMode(ctx context.Context, mode TxMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == TxModeAuto {
		if controller, ok := s.Conn.(transaction.Controller); ok && controller.InTransaction() {
			return ErrTransactionOpen
		}
	}
	s.txMode = mode
	s.lastUsed = time.Now()
	return nil
}

// CommitTransaction commits the open transaction, closing any open cursors
// first since their rows would otherwise become invalid mid-fetch.
func (s *Session) CommitTransaction(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	controller, ok := s.Conn.(transaction.Controller)
	if !ok {
		return transaction.ErrNoOpenTransaction
	}
	s.closeCursorsLocked()
	err := controller.Commit(ctx)
	// database/sql consumes a *sql.Tx on Commit/Rollback regardless of outcome
	// (see Tx.Commit godoc) — every engine here (postgres/mysql/sqlite) mirrors
	// that by unsetting its own currentTx even on error, so the driver already
	// considers the transaction gone. Clear local bookkeeping to match, or a
	// commit that fails because the connection died leaves the session
	// reporting a stale pending-statement count with no way to commit or roll
	// back it away.
	s.pendingStatements = nil
	s.lastUsed = time.Now()
	return err
}

// RollbackTransaction rolls back the open transaction, closing any open
// cursors first.
func (s *Session) RollbackTransaction(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	controller, ok := s.Conn.(transaction.Controller)
	if !ok {
		return transaction.ErrNoOpenTransaction
	}
	s.closeCursorsLocked()
	err := controller.Rollback(ctx)
	// See the matching comment in CommitTransaction: the driver's own tx
	// state is gone after this call regardless of error, so local bookkeeping
	// must follow rather than leave a stale open/pending status behind.
	s.pendingStatements = nil
	s.lastUsed = time.Now()
	return err
}

// TransactionStatus reports the session's current transaction mode, whether
// a transaction is open, and how many statements have run since the last
// commit/rollback.
func (s *Session) TransactionStatus() TransactionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	open := false
	if controller, ok := s.Conn.(transaction.Controller); ok {
		open = controller.InTransaction()
	}
	mode := s.txMode
	if mode == "" {
		mode = TxModeAuto
	}
	statements := append([]string(nil), s.pendingStatements...)
	return TransactionStatus{
		Mode:              mode,
		Open:              open,
		PendingStatements: len(statements),
		Statements:        statements,
	}
}

// Query executes a query on the session, serialized via the session mutex.
func (s *Session) Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	var rs *result.ResultSet
	err := s.runInTransaction(ctx, sql, func() error {
		var innerErr error
		rs, innerErr = s.Conn.Query(ctx, sql, args...)
		return innerErr
	})
	return rs, err
}

func (s *Session) QueryWithOptions(ctx context.Context, sql string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	var rs *result.ResultSet
	err := s.runInTransaction(ctx, sql, func() error {
		var innerErr error
		if driver, ok := s.Conn.(cursor.ResultLimitDriver); ok {
			rs, innerErr = driver.QueryWithOptions(ctx, sql, opts, args...)
		} else {
			rs, innerErr = s.Conn.Query(ctx, sql, args...)
		}
		return innerErr
	})
	return rs, err
}

// Execute executes a statement on the session, serialized via the session mutex.
func (s *Session) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	var rs *result.ResultSet
	err := s.runInTransaction(ctx, sql, func() error {
		var innerErr error
		rs, innerErr = s.Conn.Execute(ctx, sql, args...)
		return innerErr
	})
	return rs, err
}

func (s *Session) ExecuteWithOptions(ctx context.Context, sql string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	var rs *result.ResultSet
	err := s.runInTransaction(ctx, sql, func() error {
		var innerErr error
		if driver, ok := s.Conn.(cursor.ResultLimitDriver); ok {
			rs, innerErr = driver.ExecuteWithOptions(ctx, sql, opts, args...)
		} else {
			rs, innerErr = s.Conn.Execute(ctx, sql, args...)
		}
		return innerErr
	})
	return rs, err
}

// ApplyDDL applies a structured DDL operation while holding the same
// session lock used by queries and executions.
func (s *Session) ApplyDDL(ctx context.Context, request ddl.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	executor, ok := s.Conn.(ddl.Executor)
	if !ok {
		return ddl.ErrUnsupported
	}
	return s.runInTransaction(ctx, request.Summary(), func() error {
		return executor.ApplyDDL(ctx, request)
	})
}

// StartQueryCursor opens a cursor-backed query on the session. Like
// Query/Execute it runs under the session mutex and through runInTransaction,
// so a cursor-backed SELECT lazily opens a manual-mode transaction and is
// savepoint-wrapped when the driver supports savepoints.
func (s *Session) StartQueryCursor(ctx context.Context, sql string, args ...any) (*QueryCursorHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cursorDriver, ok := s.Conn.(cursor.QueryCursorDriver)
	if !ok {
		return nil, ErrQueryCursorsUnsupported
	}

	var opened cursor.QueryCursor
	err := s.runInTransaction(ctx, sql, func() error {
		var innerErr error
		opened, innerErr = cursorDriver.StartQuery(ctx, cursor.QueryRequest{SQL: sql, Args: args})
		return innerErr
	})
	if err != nil {
		return nil, err
	}

	handle := &QueryCursorHandle{ID: newULID(), Cursor: opened}

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
	handles := s.takeCursorsLocked()
	s.mu.Unlock()

	for _, handle := range handles {
		_ = handle.Close()
	}
}

// takeCursorsLocked removes every tracked cursor from the session and returns
// the handles. Must be called with s.mu held.
func (s *Session) takeCursorsLocked() []*QueryCursorHandle {
	handles := make([]*QueryCursorHandle, 0, len(s.cursors))
	for id, handle := range s.cursors {
		handles = append(handles, handle)
		delete(s.cursors, id)
	}
	return handles
}

// closeCursorsLocked closes every tracked cursor while s.mu is held, used by
// paths that must not release the session lock between resolving the
// transaction and invalidating the cursors pinned to it.
func (s *Session) closeCursorsLocked() {
	for _, handle := range s.takeCursorsLocked() {
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
	mu                sync.RWMutex
	byKey             map[string]*Session // key: "accountID:connID"
	byID              map[string]*Session // key: session ULID
	idleTimeout       time.Duration
	stop              chan struct{}
	stopped           chan struct{}
	closeOnce         sync.Once
	onConnectionEmpty func(string)
}

// SetOnConnectionEmpty registers a lifecycle hook invoked after the final live
// session for a connection is removed. The hook always runs without m.mu held.
func (m *Manager) SetOnConnectionEmpty(hook func(connectionID string)) {
	m.mu.Lock()
	m.onConnectionEmpty = hook
	m.mu.Unlock()
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
func (m *Manager) GetOrCreate(accountID, connID string, open func() (engine.Driver, func(), error)) (*Session, bool, error) {
	return m.GetOrCreateWithMetadata(accountID, connID, SessionMetadata{}, open)
}

// GetOrCreateWithMetadata returns an existing session or creates one with
// resource metadata used for workspace-scoped admin visibility and revocation.
func (m *Manager) GetOrCreateWithMetadata(accountID, connID string, metadata SessionMetadata, open func() (engine.Driver, func(), error)) (*Session, bool, error) {
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

	d, teardown, err := open()
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
		teardown:     teardown,
		lastUsed:     time.Now(),
		txMode:       TxModeAuto,
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
	// TunnelHealthy is nil when the session has no SSH tunnel; otherwise it
	// points to the tunnel's current health.
	TunnelHealthy *bool
}

// SetTunnelHealth registers a probe for this session's SSH tunnel health.
// fn returns nil when there is no tunnel. Safe to call once, right after the
// session is created.
func (s *Session) SetTunnelHealth(fn func() *bool) {
	s.mu.Lock()
	s.tunnelHealth = fn
	s.mu.Unlock()
}

// AllForAccount returns a SessionRef for every active session owned by accountID.
// It does not update lastUsed; use Get to both fetch and refresh a session.
func (m *Manager) AllForAccount(accountID string) []SessionRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var refs []SessionRef
	for _, sess := range m.byID {
		if sess.AccountID == accountID {
			refs = append(refs, sess.ref())
		}
	}
	return refs
}

// ref builds a SessionRef snapshot, probing tunnel health under s.mu.
func (s *Session) ref() SessionRef {
	s.mu.Lock()
	var tunnelHealthy *bool
	if s.tunnelHealth != nil {
		tunnelHealthy = s.tunnelHealth()
	}
	s.mu.Unlock()
	return SessionRef{
		SessionID:     s.ID,
		AccountID:     s.AccountID,
		ConnectionID:  s.ConnectionID,
		OrgID:         s.OrgID,
		WorkspaceID:   s.WorkspaceID,
		LastUsedAt:    s.lastUsed,
		TunnelHealthy: tunnelHealthy,
	}
}

// AllForWorkspace returns active sessions known to belong to workspaceID.
func (m *Manager) AllForWorkspace(workspaceID string) []SessionRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var refs []SessionRef
	for _, sess := range m.byID {
		if sess.WorkspaceID == workspaceID {
			refs = append(refs, sess.ref())
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
	sess, ok := m.byID[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}

	sess.close()
	key := sess.AccountID + ":" + sess.ConnectionID
	delete(m.byKey, key)
	delete(m.byID, sessionID)
	connectionID := sess.ConnectionID
	m.mu.Unlock()
	m.notifyConnectionEmpty(connectionID)
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
	m.mu.Unlock()
	if removed > 0 {
		m.notifyConnectionEmpty(connID)
	}
	return removed
}

// RemoveForAccount closes and removes all live sessions owned by accountID.
func (m *Manager) RemoveForAccount(accountID string) int {
	m.mu.Lock()
	connectionIDs := make(map[string]struct{})
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
		connectionIDs[sess.ConnectionID] = struct{}{}
	}
	m.mu.Unlock()
	m.notifyConnectionsEmpty(connectionIDs)
	return removed
}

// RemoveForWorkspaceAccount closes and removes all live sessions for an account
// inside one workspace.
func (m *Manager) RemoveForWorkspaceAccount(workspaceID, accountID string) int {
	m.mu.Lock()
	connectionIDs := make(map[string]struct{})
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
		connectionIDs[sess.ConnectionID] = struct{}{}
	}
	m.mu.Unlock()
	m.notifyConnectionsEmpty(connectionIDs)
	return removed
}

// RemoveForOrgAccount closes and removes all live sessions for an account in
// an organization.
func (m *Manager) RemoveForOrgAccount(orgID, accountID string) int {
	m.mu.Lock()
	connectionIDs := make(map[string]struct{})
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
		connectionIDs[sess.ConnectionID] = struct{}{}
	}
	m.mu.Unlock()
	m.notifyConnectionsEmpty(connectionIDs)
	return removed
}

// Close closes all sessions and stops the reaper goroutine. Safe to call multiple times.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
	})
	<-m.stopped

	m.mu.Lock()
	connectionIDs := make(map[string]struct{})
	for id, sess := range m.byID {
		sess.close()
		key := sess.AccountID + ":" + sess.ConnectionID
		delete(m.byKey, key)
		delete(m.byID, id)
		connectionIDs[sess.ConnectionID] = struct{}{}
	}
	m.mu.Unlock()
	m.notifyConnectionsEmpty(connectionIDs)
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
	connectionIDs := make(map[string]struct{})
	now := time.Now()
	for id, sess := range m.byID {
		if now.Sub(sess.lastUsed) > m.idleTimeout {
			sess.close()
			key := sess.AccountID + ":" + sess.ConnectionID
			delete(m.byKey, key)
			delete(m.byID, id)
			connectionIDs[sess.ConnectionID] = struct{}{}
		}
	}
	m.mu.Unlock()
	m.notifyConnectionsEmpty(connectionIDs)
}

func (m *Manager) notifyConnectionsEmpty(connectionIDs map[string]struct{}) {
	for connectionID := range connectionIDs {
		m.notifyConnectionEmpty(connectionID)
	}
}

func (m *Manager) notifyConnectionEmpty(connectionID string) {
	m.mu.RLock()
	hook := m.onConnectionEmpty
	for _, session := range m.byID {
		if session.ConnectionID == connectionID {
			m.mu.RUnlock()
			return
		}
	}
	m.mu.RUnlock()
	if hook != nil {
		hook(connectionID)
	}
}

func (s *Session) close() {
	s.mu.Lock()
	if controller, ok := s.Conn.(transaction.Controller); ok && controller.InTransaction() {
		_ = controller.Rollback(context.Background())
	}
	s.closeCursorsLocked()
	teardown := s.teardown
	s.mu.Unlock()
	_ = s.Conn.Close()
	if teardown != nil {
		teardown()
	}
}
