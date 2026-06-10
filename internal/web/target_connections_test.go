package web

import (
	"errors"
	"testing"
)

func TestValidateTargetConnectionSQLiteFilePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configure  func(*application)
		driverName string
		dsn        string
		wantErr    bool
		wantErrIs  error
	}{
		{
			name:       "server mode rejects sqlite file targets",
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
			wantErr:    true,
			wantErrIs:  errSQLiteTargetDisabled,
		},
		{
			name:       "server mode allows in-memory sqlite targets",
			driverName: "sqlite",
			dsn:        ":memory:",
		},
		{
			name:       "server mode allows sqlite shared in-memory targets",
			driverName: "sqlite",
			dsn:        "file::memory:?cache=shared",
		},
		{
			name: "local source allows sqlite file targets",
			configure: func(app *application) {
				app.config.Drivers.SQLite.AllowedSources = []string{SQLiteDriverSourceLocal}
			},
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
		},
		{
			name: "sqlite file targets are rejected when local source is not allowed",
			configure: func(app *application) {
				app.config.DeploymentMode = DeploymentModeDesktop
				app.config.AccessMode = AccessModeSingleUser
				app.config.Drivers.SQLite.AllowedSources = nil
			},
			driverName: "sqlite",
			dsn:        "/tmp/customer.db",
			wantErr:    true,
			wantErrIs:  errSQLiteTargetDisabled,
		},
		{
			name:       "non-sqlite registered drivers are unaffected",
			driverName: "postgres",
			dsn:        "host=localhost user=test dbname=test",
		},
		{
			name:       "unknown driver remains unsupported",
			driverName: "oracle",
			dsn:        "example",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			if tt.configure != nil {
				tt.configure(app)
			}

			err := app.validateTargetConnection(tt.driverName, tt.dsn)
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
