package dbengine

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/pkg/result"
)

type fakeDriver struct{}

func (fakeDriver) Connect(context.Context, ConnectionConfig) error { return nil }
func (fakeDriver) Ping(context.Context) error                      { return nil }
func (fakeDriver) Close() error                                    { return nil }
func (fakeDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (fakeDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (fakeDriver) Dialect() Dialect { return DialectPostgres }

func resetRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	registry = map[EngineID]Registration{}
	registryMu.Unlock()
}

func registerFake(id EngineID, dialect Dialect) {
	Register(Registration{
		ID:          id,
		DisplayName: string(id),
		Dialect:     dialect,
		New:         func() Driver { return fakeDriver{} },
	})
}

func TestNewReturnsNonConnectedDriver(t *testing.T) {
	resetRegistry(t)
	registerFake("postgres", DialectPostgres)

	d, err := New("postgres")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if d == nil {
		t.Fatal("New returned a nil driver")
	}
	if d.Dialect() != DialectPostgres {
		t.Fatalf("dialect = %q, want postgres", d.Dialect())
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

func TestDescribeReturnsMetadata(t *testing.T) {
	resetRegistry(t)
	registerFake("postgres", DialectPostgres)

	set, ok := Describe("postgresql") // alias resolves
	if !ok {
		t.Fatal("Describe should find postgres via alias")
	}
	if set.Engine.ID != "postgres" || set.Engine.Dialect != DialectPostgres {
		t.Fatalf("unexpected descriptor: %+v", set.Engine)
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
		if got[i].Engine.ID != id {
			t.Fatalf("position %d = %q, want %q", i, got[i].Engine.ID, id)
		}
	}
}
