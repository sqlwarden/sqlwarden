package connection

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"
)

// mockDriver is a test double for driver.Driver that tracks calls.
type mockDriver struct {
	mu     sync.Mutex
	closed bool
	pings  int
}

func (d *mockDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error { return nil }
func (d *mockDriver) Ping(ctx context.Context) error {
	d.mu.Lock()
	d.pings++
	d.mu.Unlock()
	return nil
}
func (d *mockDriver) Close() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	return nil
}
func (d *mockDriver) Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (d *mockDriver) Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (d *mockDriver) Tables(ctx context.Context, database, schema string) ([]driver.TableMeta, error) {
	return nil, nil
}
func (d *mockDriver) Columns(ctx context.Context, database, schema, table string) ([]driver.ColumnMeta, error) {
	return nil, nil
}
func (d *mockDriver) Dialect() driver.Dialect { return driver.DialectSQLite }

func newMockOpen() (driver.Driver, func() bool) {
	d := &mockDriver{}
	return d, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.closed
	}
}

// TestReuse verifies that calling GetOrCreate twice for the same account+conn returns the same session.
func TestReuse(t *testing.T) {
	m := New(5 * time.Minute)
	defer m.Close()

	calls := 0
	open := func() (driver.Driver, error) {
		calls++
		d := &mockDriver{}
		return d, nil
	}

	sess1, created1, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created1 {
		t.Fatal("expected created=true on first call")
	}

	sess2, created2, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created2 {
		t.Fatal("expected created=false on second call")
	}

	if sess1.ID != sess2.ID {
		t.Fatalf("expected same session ID, got %s and %s", sess1.ID, sess2.ID)
	}

	if calls != 1 {
		t.Fatalf("expected open() called once, got %d", calls)
	}
}

// TestIsolation verifies that different accounts get different sessions for the same connID.
func TestIsolation(t *testing.T) {
	m := New(5 * time.Minute)
	defer m.Close()

	open := func() (driver.Driver, error) {
		return &mockDriver{}, nil
	}

	sessAlice, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sessBob, _, err := m.GetOrCreate("bob", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sessAlice.ID == sessBob.ID {
		t.Fatal("expected different sessions for different accounts")
	}
}

// TestGetByID verifies that Get returns the correct session by ID.
func TestGetByID(t *testing.T) {
	m := New(5 * time.Minute)
	defer m.Close()

	open := func() (driver.Driver, error) {
		return &mockDriver{}, nil
	}

	sess, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, ok := m.Get(sess.ID)
	if !ok {
		t.Fatal("expected to find session by ID")
	}
	if got.ID != sess.ID {
		t.Fatalf("expected session ID %s, got %s", sess.ID, got.ID)
	}
}

// TestGetUnknown verifies that Get returns (nil, false) for an unknown session ID.
func TestGetUnknown(t *testing.T) {
	m := New(5 * time.Minute)
	defer m.Close()

	got, ok := m.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for unknown session")
	}
	if got != nil {
		t.Fatal("expected nil for unknown session")
	}
}

// TestReapIdle verifies that sessions exceeding the idle timeout are reaped.
func TestReapIdle(t *testing.T) {
	m := New(100 * time.Millisecond)
	defer m.Close()

	md := &mockDriver{}
	open := func() (driver.Driver, error) {
		return md, nil
	}

	sess, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for idle timeout to expire.
	time.Sleep(200 * time.Millisecond)

	// Trigger reap manually.
	m.reapIdle()

	// Driver should be closed.
	md.mu.Lock()
	closed := md.closed
	md.mu.Unlock()
	if !closed {
		t.Fatal("expected driver to be closed after reap")
	}

	// Session should no longer be findable.
	_, ok := m.Get(sess.ID)
	if ok {
		t.Fatal("expected session to be gone after reap")
	}
}

// TestClose verifies that Close closes all sessions.
func TestClose(t *testing.T) {
	m := New(5 * time.Minute)

	md := &mockDriver{}
	open := func() (driver.Driver, error) {
		return md, nil
	}

	sess, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.Close()

	// Driver should be closed.
	md.mu.Lock()
	closed := md.closed
	md.mu.Unlock()
	if !closed {
		t.Fatal("expected driver to be closed after manager Close")
	}

	// Session should no longer be findable.
	_, ok := m.Get(sess.ID)
	if ok {
		t.Fatal("expected session to be gone after Close")
	}
}

// TestRemove verifies that Remove closes the driver and removes the session.
func TestRemove(t *testing.T) {
	m := New(5 * time.Minute)
	defer m.Close()

	md := &mockDriver{}
	open := func() (driver.Driver, error) {
		return md, nil
	}

	sess, _, err := m.GetOrCreate("alice", "conn1", open)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m.Remove(sess.ID)

	// Driver should be closed.
	md.mu.Lock()
	closed := md.closed
	md.mu.Unlock()
	if !closed {
		t.Fatal("expected driver to be closed after Remove")
	}

	// Session should no longer be findable.
	_, ok := m.Get(sess.ID)
	if ok {
		t.Fatal("expected session to be gone after Remove")
	}
}
