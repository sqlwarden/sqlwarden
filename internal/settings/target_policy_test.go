package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/database"

	_ "github.com/sqlwarden/internal/engine/engines/postgres"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
)

type fakeStore struct {
	settings database.InstanceSettings
	found    bool
	err      error
}

func (s fakeStore) GetInstanceSettings(context.Context) (database.InstanceSettings, bool, error) {
	return s.settings, s.found, s.err
}

func TestTargetPolicyCheck(t *testing.T) {
	allowAll := database.InstanceSettings{SQLiteLocalTargetsEnabled: true, SQLiteInMemoryTargetsEnabled: true}
	tests := []struct {
		name   string
		store  InstanceSettingsStore
		driver string
		dsn    string
		want   error
	}{
		{name: "sqlite file allowed", store: fakeStore{settings: allowAll, found: true}, driver: "sqlite", dsn: "/tmp/a.db"},
		{name: "sqlite file disabled", store: fakeStore{settings: database.InstanceSettings{SQLiteInMemoryTargetsEnabled: true}, found: true}, driver: "sqlite", dsn: "/tmp/a.db", want: ErrSQLiteFileTargetDisabled},
		{name: "in-memory disabled", store: fakeStore{settings: database.InstanceSettings{SQLiteLocalTargetsEnabled: true}, found: true}, driver: " sqlite ", dsn: " file::memory:?cache=shared", want: ErrSQLiteInMemoryTargetDisabled},
		{name: "unknown driver", store: fakeStore{found: true}, driver: "db2", dsn: "x", want: ErrUnsupportedDriver},
		{name: "non-sqlite ignores settings", store: fakeStore{err: errors.New("down")}, driver: "postgres", dsn: "host=x"},
		{name: "settings error denies sqlite", store: fakeStore{err: errors.New("down")}, driver: "sqlite", dsn: ":memory:", want: ErrTargetDenied},
		{name: "missing row denies sqlite", store: fakeStore{}, driver: "sqlite", dsn: ":memory:", want: ErrTargetDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewTargetPolicy(tt.store).Check(context.Background(), tt.driver, tt.dsn)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("Check: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) || !errors.Is(err, ErrTargetDenied) {
				t.Fatalf("Check error = %v, want %v wrapping ErrTargetDenied", err, tt.want)
			}
		})
	}
}
