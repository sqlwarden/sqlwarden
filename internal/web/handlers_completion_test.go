package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestCompleteConnectionSQLFromPersistentSnapshot(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Completion DB", "open")

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"}, metadata.ScopeSegment{Kind: "schema", Name: "public"})
	directory := &metadata.Directory{
		Engine: "postgres", DefaultScope: scope, GeneratedAt: time.Now(),
		Roots: []metadata.ScopeNode{{
			Path: scope,
			Groups: []metadata.ObjectGroup{{
				Kind: "table",
				Objects: []metadata.ObjectRef{
					{Scope: scope, Kind: "table", Name: "widgets"},
				},
			}},
		}},
	}
	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.PutObjects(context.Background(), snapshot.ID, []metadata.Object{{
		Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "widgets"},
		Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
			{Name: "widget_name", DataType: "text", Ordinal: 1},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
		t.Fatal(err)
	}

	sql := "SELECT * FROM wid"
	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/completion",
		map[string]any{"sql": sql, "cursor_offset": len(sql)}, token)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["mode"], "persistent")
	assert.Equal(t, res.BodyFields["metadata_available"], true)
	assert.Equal(t, res.BodyFields["snapshot_id"], any(snapshot.ID))
	if !responseHasCompletionLabel(res.BodyFields, "widgets") {
		t.Fatalf("expected widgets completion, got %s", res.BodyBytes)
	}
}

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

func TestCompleteConnectionSQLFromEphemeralSessionMetadata(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "completion-ephemeral-ready"), "Completion", "Completion Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Completion WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "postgres", "Completion DB", "open")
	session := openSchemaSession(t, app, owner.ID, conn.ID, schemaFakeDriver{})

	sql := "SELECT * FROM wid"
	req := newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/completion",
		map[string]any{"sql": sql, "cursor_offset": len(sql)}, token)
	req.Header.Set("X-Warden-Session", session.ID)
	res := send(t, req, app.routes())

	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["mode"], "ephemeral")
	assert.Equal(t, res.BodyFields["metadata_available"], true)
	assert.Equal(t, res.BodyFields["metadata_status"], "ready")
	if !responseHasCompletionLabel(res.BodyFields, "widgets") {
		t.Fatalf("expected widgets completion from ephemeral metadata, got %s", res.BodyBytes)
	}
}

func TestCompleteConnectionSQLRejectsInvalidOffsetsAndUnsupportedSQLite(t *testing.T) {
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
	assert.Equal(t, res.StatusCode, http.StatusNotImplemented)
}

func TestCompleteConnectionSQLRejectsSessionFromAnotherConnection(t *testing.T) {
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
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
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
