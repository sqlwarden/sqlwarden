package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func TestCompleteConnectionSQLKeywordOnlyWithoutEphemeralSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-ephemeral"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Completion DB", "open")
	if err := app.db.UpdateConnectionWithPolicy(context.Background(), conn.ID, conn.Name, conn.DSNEncrypted, conn.AccessMode, database.SchemaSnapshotPolicyDisabled); err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/completion",
		map[string]any{"sql": "SEL", "cursor_offset": 3}, token)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["mode"], "ephemeral")
	assert.Equal(t, res.BodyFields["metadata_available"], false)
	if !responseHasCompletionLabel(res.BodyFields, "SELECT") {
		t.Fatalf("expected SELECT completion, got %s", res.BodyBytes)
	}

	automatic := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/completion",
		map[string]any{
			"sql": "SELECT ", "cursor_offset": 7,
			"trigger_kind": "automatic", "trigger_character": " ",
		}, token)
	automaticRes := send(t, automatic, app.routes())
	assert.Equal(t, automaticRes.StatusCode, http.StatusOK)
	suggestions := automaticRes.BodyFields["suggestions"].([]any)
	assert.Equal(t, len(suggestions), 10)
	if responseHasCompletionLabel(automaticRes.BodyFields, "ALTER") {
		t.Fatalf("automatic bare SELECT leaked unrelated grammar candidates: %s", automaticRes.BodyBytes)
	}
}

func TestCompleteConnectionSQLRejectsInvalidOffsetsAndSupportsSQLite(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-invalid"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	pg := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "PG", "open")
	sqlite := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "SQLite", "open")

	for _, offset := range []int{-1, 1, 99} {
		req := newAuthRequest(t, http.MethodPost,
			orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(pg.ID, 10))+"/completion",
			map[string]any{"sql": "😀", "cursor_offset": offset}, token)
		res := send(t, req, app.routes())
		assert.Equal(t, res.StatusCode, http.StatusBadRequest)
	}

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(sqlite.ID, 10))+"/completion",
		map[string]any{"sql": "SEL", "cursor_offset": 3}, token)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	if !responseHasCompletionLabel(res.BodyFields, "SELECT") {
		t.Fatalf("expected SELECT keyword completion for sqlite, got %s", res.BodyBytes)
	}
}

func TestCompleteConnectionSQLIgnoresSessionFromAnotherConnection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-scope"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	target := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Target", "open")
	other := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Other", "open")
	disableSchemaSnapshots(t, app, target.ID)
	session := openSchemaSession(t, app, owner.ID, other.ID, schemaFakeDriver{})

	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(target.ID, 10))+"/completion",
		map[string]any{"sql": "SEL", "cursor_offset": 3}, token)
	req.Header.Set("X-Warden-Session", session.ID)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["metadata_loaded"], false)
}

func responseHasCompletionLabel(body map[string]any, label string) bool {
	suggestions, _ := body["suggestions"].([]any)
	for _, raw := range suggestions {
		suggestion, _ := raw.(map[string]any)
		if suggestion["label"] == label {
			return true
		}
	}
	return false
}
