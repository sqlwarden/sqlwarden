package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
)

func TestDownloadConnectionExportStreamsCSVFromLiveSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "export-sync"), "Export Sync", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Export Sync WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "ExportSyncConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	baseURL := orgConnectionURL(slug, wsIDInt, envID, connID)

	connectRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	req := newAuthRequest(t, http.MethodPost, baseURL+"/exports/download", map[string]any{
		"sql":      "SELECT 1 AS id, 'alpha' AS name",
		"filename": "report",
	}, tok)
	req.Header.Set("X-Warden-Session", sessionID)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.Header.Get("Content-Type"), "text/csv; charset=utf-8")
	assert.True(t, strings.Contains(res.Header.Get("Content-Disposition"), "report.csv"))
	assert.Equal(t, string(res.BodyBytes), "id,name\n1,alpha\n")
}

func TestDownloadConnectionExportAbortsOnByteLimitExceeded(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.config.Exports.SyncMaxBytes = 5

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "export-overflow"), "Export Overflow", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Export Overflow WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)

	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "ExportOverflowConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	baseURL := orgConnectionURL(slug, wsIDInt, envID, connID)

	connectRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/connect", nil, tok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	req := newAuthRequest(t, http.MethodPost, baseURL+"/exports/download", map[string]any{
		"sql":      "SELECT 1 AS id, 'alpha' AS name",
		"filename": "overflow",
	}, tok)
	req.Header.Set("X-Warden-Session", sessionID)

	rec := httptest.NewRecorder()
	var panicked any
	func() {
		defer func() { panicked = recover() }()
		app.routes().ServeHTTP(rec, req)
	}()

	gotErr, ok := panicked.(error)
	assert.True(t, ok)
	assert.ErrorIs(t, gotErr, http.ErrAbortHandler)
}

func TestCreateConnectionExportQueuesUserJob(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "export-job"), "Export Job", "Export Job Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Job WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "ExportJobConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, connID)+"/exports",
		map[string]any{"sql": "SELECT 1 AS id", "filename": "queued"}, tok), app.routes())

	assert.Equal(t, res.StatusCode, http.StatusCreated)
	var job jobs.Record
	decodeJSONResponse(t, res.BodyBytes, &job)
	if err := app.db.NewSelect().Model(&job).Where("id = ?", job.ID).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, job.Type, jobs.TypeExportQueryCSV)
	assert.Equal(t, job.Visibility, jobs.VisibilityUser)
	assert.Equal(t, *job.OrgID, org.ID)
	assert.Equal(t, *job.WorkspaceID, ws.ID)
	assert.Equal(t, *job.OwnerAccountID, account.ID)
	var input exportJobInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, input.Filename, "queued")
	assert.Equal(t, input.Format, "csv")
}

func TestConnectionExportRejectsNonReadQuery(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "export-nondql"), "Export Non DQL", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Export Non DQL WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "ExportNonDQLConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/exports",
		map[string]any{"sql": "CREATE TABLE exports_reject (id INTEGER)"}, tok), app.routes())

	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "sql")
}

func TestConnectionExportRejectsMultiStatementQuery(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "export-multi"), "Export Multi", "securepass99")
	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Export Multi WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])
	wsIDInt, _ := strconv.ParseInt(wsID, 10, 64)
	envID := defaultEnvironmentID(t, app, wsIDInt)
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(slug, wsIDInt, envID),
		map[string]any{"name": "ExportMultiConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/exports",
		map[string]any{"sql": "SELECT 1 AS id; SELECT 2 AS id"}, tok), app.routes())

	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "sql")

	downloadRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(slug, wsIDInt, envID, connID)+"/exports/download",
		map[string]any{"sql": "SELECT 1 AS id; SELECT 2 AS id"}, tok), app.routes())
	assert.Equal(t, downloadRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, downloadRes, "sql")
}

func TestHandleExportJobWritesPrivateWorkspaceFile(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "export-worker"), "Export Worker", "Export Worker Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Export Worker WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	createRes := send(t, newAuthRequest(t, http.MethodPost,
		orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "ExportWorkerConn", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
	connIDInt, _ := strconv.ParseInt(connID, 10, 64)

	input := exportJobInput{
		AccountID:    account.ID,
		OrgID:        org.ID,
		WorkspaceID:  ws.ID,
		ConnectionID: connIDInt,
		SQL:          "SELECT 7 AS id, 'exported' AS name",
		Format:       "csv",
		Filename:     "worker-export",
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := app.handleExportJob(context.Background(), jobs.Runtime{
		Job:    jobs.Record{ID: database.NewID(), Type: jobs.TypeExportQueryCSV, InputJSON: string(inputJSON)},
		Events: recordingEventWriter{},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := output.(exportJobOutput)
	assert.Equal(t, result.Filename, "worker-export.csv")
	assert.Equal(t, result.RowCount, int64(1))

	scope := files.Scope{AccountID: account.ID, OrgID: org.ID, OrgSlug: org.Slug, Workspace: ws, Visibility: database.FileVisibilityPrivate}
	content, err := app.workspaceFileService().ReadContent(context.Background(), scope, result.FileID)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Reader.Close()
	body, err := io.ReadAll(content.Reader)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, string(body), "id,name\n7,exported\n")
}

type recordingEventWriter struct{}

func (recordingEventWriter) Info(context.Context, string, string, any)  {}
func (recordingEventWriter) Warn(context.Context, string, string, any)  {}
func (recordingEventWriter) Error(context.Context, string, string, any) {}
