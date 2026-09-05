package web

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func TestValidateTargetConnectionSQLiteFilePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configure  func(*testing.T, *application)
		driverName string
		dsn        string
		wantErr    bool
		wantErrIs  error
	}{
		{
			name:       "default instance allows sqlite file targets",
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
		},
		{
			name: "disabled instance rejects sqlite file targets",
			configure: func(t *testing.T, app *application) {
				updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
					settings.SQLiteLocalTargetsEnabled = false
				})
			},
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
			wantErr:    true,
			wantErrIs:  errSQLiteTargetDisabled,
		},
		{
			name: "in-memory sqlite targets allowed when file targets are disabled",
			configure: func(t *testing.T, app *application) {
				updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
					settings.SQLiteLocalTargetsEnabled = false
				})
			},
			driverName: "sqlite",
			dsn:        ":memory:",
		},
		{
			name:       "in-memory sqlite targets are allowed when enabled",
			driverName: "sqlite",
			dsn:        ":memory:",
		},
		{
			name:       "shared in-memory sqlite targets are allowed when enabled",
			driverName: "sqlite",
			dsn:        "file::memory:?cache=shared",
		},
		{
			name: "disabled instance rejects in-memory sqlite targets",
			configure: func(t *testing.T, app *application) {
				updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
					settings.SQLiteInMemoryTargetsEnabled = false
				})
			},
			driverName: "sqlite",
			dsn:        ":memory:",
			wantErr:    true,
			wantErrIs:  errSQLiteInMemoryTargetDisabled,
		},
		{
			name: "disabled instance rejects shared in-memory sqlite targets",
			configure: func(t *testing.T, app *application) {
				updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
					settings.SQLiteInMemoryTargetsEnabled = false
				})
			},
			driverName: "sqlite",
			dsn:        "file::memory:?cache=shared",
			wantErr:    true,
			wantErrIs:  errSQLiteInMemoryTargetDisabled,
		},
		{
			name: "file targets still allowed when in-memory targets are disabled",
			configure: func(t *testing.T, app *application) {
				updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
					settings.SQLiteInMemoryTargetsEnabled = false
				})
			},
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
		},
		{
			name:       "non-sqlite registered drivers are unaffected",
			driverName: "postgres",
			dsn:        "host=localhost user=test dbname=test",
		},
		{
			name:       "unknown driver remains unsupported",
			driverName: "db2",
			dsn:        "example",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			if tt.configure != nil {
				tt.configure(t, app)
			}

			err := app.validateTargetConnection(context.Background(), tt.driverName, tt.dsn)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("validateTargetConnection returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validateTargetConnection returned nil error")
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("validateTargetConnection error = %v, want %v", err, tt.wantErrIs)
			}
		})
	}
}
