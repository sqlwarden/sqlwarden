package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/jobs"
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
			wantErrIs:  execution.ErrTargetRejected,
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

type openFailureRuntime struct {
	execution.SessionRuntime
	err error
}

func (r openFailureRuntime) Open(context.Context, execution.OpenRequest) (execution.OpenResult, error) {
	return execution.OpenResult{}, r.err
}

const openFailureSecretMarker = "postgres://user:open-secret-marker@db/app"

var targetOpenFailureCases = []struct {
	name          string
	err           error
	wantRetryable bool
}{
	{name: "credentials not found", err: fmt.Errorf("%w: %s", execution.ErrCredentialsNotFound, openFailureSecretMarker)},
	{name: "credential decryption", err: fmt.Errorf("%w: %s", execution.ErrCredentialDecryption, openFailureSecretMarker)},
	{name: "credentials invalid", err: errors.Join(execution.ErrCredentialsInvalid, errors.New(openFailureSecretMarker))},
	{name: "target connection", err: fmt.Errorf("%w: %s", execution.ErrTargetConnection, openFailureSecretMarker), wantRetryable: true},
	{name: "transport", err: errors.New(openFailureSecretMarker), wantRetryable: true},
}

func assertTargetOpenJobError(t *testing.T, err error, permanentCode, retryableCode string, wantRetryable bool) {
	t.Helper()
	var coded jobs.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want jobs.CodedError", err)
	}
	wantCode := permanentCode
	if wantRetryable {
		wantCode = retryableCode
	}
	assert.Equal(t, coded.Code, wantCode)
	assert.Equal(t, coded.Retryable, wantRetryable)
	if strings.Contains(coded.Message, "open-secret-marker") {
		t.Fatalf("job error leaked underlying error: %q", coded.Message)
	}
}

func TestHandleExportJobClassifiesTargetOpenFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range targetOpenFailureCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			account, _, org := seedOrgOwner(t, app, uniqueEmail(t, "export-open-failure"), "Export Open Failure", "Export Open Failure Org")
			ws := seedWorkspaceForAccount(t, app, org, account, "Export Open Failure WS", "")
			envID := defaultEnvironmentID(t, app, ws.ID)
			conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Export Open Failure Conn", "open")
			app.executionRuntime = openFailureRuntime{SessionRuntime: app.executionRuntime, err: tt.err}

			_, err := runExportJob(t, app, exportJobInput{
				AccountID: account.ID, OrgID: org.ID, WorkspaceID: ws.ID, ConnectionID: conn.ID,
				SQL: "SELECT 1 AS id", Format: "csv", Filename: "open-failure",
			})
			assertTargetOpenJobError(t, err, "export_credentials_unavailable", "export_connect_failed", tt.wantRetryable)
		})
	}
}

func TestHandleExportJobTreatsUndecryptableStoredCredentialsAsPermanent(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	account, _, org := seedOrgOwner(t, app, uniqueEmail(t, "export-undecryptable"), "Export Undecryptable", "Export Undecryptable Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Undecryptable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Export Undecryptable Conn", "open")

	_, err := runExportJob(t, app, exportJobInput{
		AccountID: account.ID, OrgID: org.ID, WorkspaceID: ws.ID, ConnectionID: conn.ID,
		SQL: "SELECT 1 AS id", Format: "csv", Filename: "undecryptable",
	})
	assertTargetOpenJobError(t, err, "export_credentials_unavailable", "export_connect_failed", false)
}

func TestOpenTargetSchemaInspectorClassifiesTargetOpenFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range targetOpenFailureCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app := newTestApp(t)
			owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-open-failure"), "Schema Open Failure", "Schema Open Failure Org")
			ws := seedWorkspaceForAccount(t, app, org, owner, "Schema Open Failure WS", "")
			envID := defaultEnvironmentID(t, app, ws.ID)
			conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Open Failure Conn", "open")
			app.executionRuntime = openFailureRuntime{SessionRuntime: app.executionRuntime, err: tt.err}

			_, _, _, err := app.openTargetSchemaInspector(context.Background(), conn, ws)
			assertTargetOpenJobError(t, err, "schema_sync_credentials_unavailable", "schema_sync_connect_failed", tt.wantRetryable)
		})
	}
}

func TestOpenTargetSchemaInspectorTreatsUndecryptableStoredCredentialsAsPermanent(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "schema-undecryptable"), "Schema Undecryptable", "Schema Undecryptable Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema Undecryptable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Undecryptable Conn", "open")

	_, _, _, err := app.openTargetSchemaInspector(context.Background(), conn, ws)
	assertTargetOpenJobError(t, err, "schema_sync_credentials_unavailable", "schema_sync_connect_failed", false)
}

func runExportJob(t *testing.T, app *application, input exportJobInput) (any, error) {
	t.Helper()
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return app.handleExportJob(context.Background(), jobs.Runtime{
		Job:    jobs.Record{ID: database.NewID(), Type: jobs.TypeExportQueryCSV, InputJSON: string(inputJSON)},
		Events: recordingEventWriter{},
	})
}
