package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
)

func TestConnectionExportsUnavailableWithoutRegisteredClassifier(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "export-unavailable"), "Export Unavailable", "Export Unavailable Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Unavailable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "ExportUnavailableConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	baseURL := orgConnectionURL(org.Slug, ws.ID, envID, connID)

	for _, path := range []string{"/exports", "/exports/download"} {
		res := send(t, newAuthRequest(t, http.MethodPost, baseURL+path,
			map[string]any{"sql": "SELECT 1 AS id", "filename": "report"}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
	}

	count, err := app.db.NewSelect().Model((*jobs.Record)(nil)).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 0)
}

func TestHandleExportJobFailsBeforeTargetAccessWithoutRegisteredClassifier(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, _, org := seedOrgOwner(t, app, uniqueEmail(t, "export-worker-unavailable"), "Export Worker Unavailable", "Export Worker Unavailable Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Worker Unavailable WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Export Worker Unavailable Conn")

	inputJSON, err := json.Marshal(exportJobInput{
		AccountID:    account.ID,
		OrgID:        org.ID,
		WorkspaceID:  ws.ID,
		ConnectionID: conn.ID,
		SQL:          "SELECT 7 AS id",
		Format:       "csv",
		Filename:     "worker-export",
	})
	if err != nil {
		t.Fatal(err)
	}

	output, err := app.handleExportJob(context.Background(), jobs.Runtime{
		Job:    jobs.Record{ID: database.NewID(), Type: jobs.TypeExportQueryCSV, InputJSON: string(inputJSON)},
		Events: recordingEventWriter{},
	})
	if output != nil {
		t.Fatalf("output = %#v, want nil", output)
	}
	var coded jobs.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want jobs.CodedError", err)
	}
	assert.Equal(t, coded.Code, "export_classifier_unavailable")
	assert.Equal(t, coded.Retryable, false)
}

func TestRegisteredClassifierIsRequiredForExportValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	ok := app.validateExportSQL(recorder, req, database.Connection{Driver: "sqlite"}, "SELECT 1")
	assert.Equal(t, ok, false)
	assert.Equal(t, recorder.Code, http.StatusNotImplemented)
}

func TestOmniClassifierExportValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	tests := []struct {
		name       string
		driver     string
		sql        string
		wantOK     bool
		wantStatus int
	}{
		{name: "postgres read", driver: "postgres", sql: "SELECT 1", wantOK: true},
		{name: "mysql read", driver: "mysql", sql: "SHOW TABLES", wantOK: true},
		{name: "postgres read CTE", driver: "postgres", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value", wantOK: true},
		{name: "mysql read CTE", driver: "mysql", sql: "WITH value AS (SELECT 1 AS n) SELECT n FROM value", wantOK: true},
		{name: "postgres mutation", driver: "postgres", sql: "DELETE FROM widgets", wantStatus: http.StatusUnprocessableEntity},
		{name: "mysql locking read", driver: "mysql", sql: "SELECT * FROM widgets FOR UPDATE", wantStatus: http.StatusUnprocessableEntity},
		{name: "postgres multi query", driver: "postgres", sql: "SELECT 1; SELECT 2", wantStatus: http.StatusUnprocessableEntity},
		{name: "mysql invalid", driver: "mysql", sql: "SELECT FROM", wantStatus: http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			got := app.validateExportSQL(recorder, req, database.Connection{Driver: tt.driver}, tt.sql)
			if got != tt.wantOK {
				t.Fatalf("validateExportSQL() = %v, want %v", got, tt.wantOK)
			}
			if !tt.wantOK && recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

type recordingEventWriter struct{}

func (recordingEventWriter) Info(context.Context, string, string, any)  {}
func (recordingEventWriter) Warn(context.Context, string, string, any)  {}
func (recordingEventWriter) Error(context.Context, string, string, any) {}
