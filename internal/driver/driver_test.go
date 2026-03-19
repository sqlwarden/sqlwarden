package driver

import (
	"context"
	"testing"

	"github.com/sqlwarden/pkg/result"
)

// mockDriver is a minimal Driver implementation for testing.
type mockDriver struct{}

func (m *mockDriver) Connect(_ context.Context, _ ConnectionConfig) error { return nil }
func (m *mockDriver) Ping(_ context.Context) error                        { return nil }
func (m *mockDriver) Close() error                                        { return nil }
func (m *mockDriver) Query(_ context.Context, _ string, _ ...any) (*result.ResultSet, error) {
	return nil, nil
}
func (m *mockDriver) Execute(_ context.Context, _ string, _ ...any) (*result.ResultSet, error) {
	return nil, nil
}
func (m *mockDriver) Tables(_ context.Context, _, _ string) ([]TableMeta, error) { return nil, nil }
func (m *mockDriver) Columns(_ context.Context, _, _, _ string) ([]ColumnMeta, error) {
	return nil, nil
}
func (m *mockDriver) Dialect() Dialect { return DialectPostgres }

// resetRegistry clears the registry map directly (same package).
func resetRegistry() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]func() Driver{}
}

func TestRegisterAndNew(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	Register("mock-register-new", func() Driver { return &mockDriver{} })

	d, err := New("mock-register-new")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil Driver instance")
	}
}

func TestNewUnknownDriver(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	_, err := New("nonexistent-driver")
	if err == nil {
		t.Fatal("expected error for unknown driver, got nil")
	}
}

func TestAliasResolution(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	Register("postgres", func() Driver { return &mockDriver{} })
	Register("mysql", func() Driver { return &mockDriver{} })
	Register("sqlite", func() Driver { return &mockDriver{} })

	cases := []struct {
		alias    string
		resolved string
	}{
		{"postgresql", "postgres"},
		{"sqlite3", "sqlite"},
		{"mariadb", "mysql"},
	}

	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			d, err := New(tc.alias)
			if err != nil {
				t.Fatalf("alias %q: expected no error, got %v", tc.alias, err)
			}
			if d == nil {
				t.Fatalf("alias %q: expected non-nil Driver", tc.alias)
			}
		})
	}
}

func TestRegisterPanicOnDuplicate(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	Register("mock-dup", func() Driver { return &mockDriver{} })

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
	}()

	Register("mock-dup", func() Driver { return &mockDriver{} })
}

func TestRegisterPanicOnEmptyName(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on empty name, got none")
		}
	}()

	Register("", func() Driver { return &mockDriver{} })
}
