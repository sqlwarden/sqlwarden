package dbengine

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"
)

type fakeDriver struct{}

func (fakeDriver) Connect(context.Context, driver.ConnectionConfig) error { return nil }
func (fakeDriver) Ping(context.Context) error                             { return nil }
func (fakeDriver) Close() error                                           { return nil }
func (fakeDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (fakeDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (fakeDriver) Dialect() driver.Dialect { return driver.DialectPostgres }

func resetRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	registry = map[EngineID]*facadeEngine{}
	registryMu.Unlock()
}

func registerFake(id EngineID, dialect Dialect) {
	Register(Registration{
		ID:          id,
		DisplayName: string(id),
		Dialect:     dialect,
		NewDriver:   func() driver.Driver { return fakeDriver{} },
	})
}

func TestRegisterAndNew(t *testing.T) {
	resetRegistry(t)
	registerFake("postgres", DialectPostgres)

	eng, err := New("postgres")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if eng.ID() != "postgres" || eng.Dialect() != DialectPostgres {
		t.Fatalf("unexpected engine: id=%q dialect=%q", eng.ID(), eng.Dialect())
	}
}

func TestNewNormalizesAlias(t *testing.T) {
	resetRegistry(t)
	registerFake("postgres", DialectPostgres)

	if _, err := New("postgresql"); err != nil {
		t.Fatalf("alias postgresql should resolve to postgres: %v", err)
	}
}

func TestNewUnknownReturnsTypedError(t *testing.T) {
	resetRegistry(t)
	if _, err := New("nope"); !errors.Is(err, ErrUnknownEngine) {
		t.Fatalf("want ErrUnknownEngine, got %v", err)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	resetRegistry(t)
	registerFake("postgres", DialectPostgres)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate Register should panic")
		}
	}()
	registerFake("postgres", DialectPostgres)
}

func TestEnginesSortedByID(t *testing.T) {
	resetRegistry(t)
	registerFake("sqlite", DialectSQLite)
	registerFake("mysql", DialectMySQL)
	registerFake("postgres", DialectPostgres)

	got := Engines()
	want := []EngineID{"mysql", "postgres", "sqlite"}
	if len(got) != len(want) {
		t.Fatalf("got %d engines, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID() != id {
			t.Fatalf("position %d = %q, want %q", i, got[i].ID(), id)
		}
	}
}
