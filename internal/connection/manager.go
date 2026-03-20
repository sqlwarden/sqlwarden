package connection

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"
)

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
	ID           string        // ULID
	AccountID    string
	ConnectionID string
	Driver       driver.Driver // open connection
	mu           sync.Mutex    // serializes Query/Execute on this session
	lastUsed     time.Time
}

// Query executes a query on the session, serialized via the session mutex.
func (s *Session) Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	return s.Driver.Query(ctx, sql, args...)
}

// Execute executes a statement on the session, serialized via the session mutex.
func (s *Session) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()
	return s.Driver.Execute(ctx, sql, args...)
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
func (m *Manager) GetOrCreate(accountID, connID string, open func() (driver.Driver, error)) (*Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", accountID, connID)
	if sess, ok := m.byKey[key]; ok {
		sess.lastUsed = time.Now()
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
		Driver:       d,
		lastUsed:     time.Now(),
	}

	m.byKey[key] = sess
	m.byID[sess.ID] = sess

	return sess, true, nil
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

	sess.Driver.Close()
	key := sess.AccountID + ":" + sess.ConnectionID
	delete(m.byKey, key)
	delete(m.byID, sessionID)
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
		sess.Driver.Close()
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
			sess.Driver.Close()
			key := sess.AccountID + ":" + sess.ConnectionID
			delete(m.byKey, key)
			delete(m.byID, id)
		}
	}
}
