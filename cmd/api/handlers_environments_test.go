package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestEnvironmentLifecycle(t *testing.T) {
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "envowner@example.com", "Env Owner", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Main WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	// Create environment.
	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging", "description": "Staging env"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	// Get environment.
	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "staging")

	// List environments.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	// Update environment.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		map[string]any{"name": "production", "description": "Production env"}, tok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)

	// Delete environment.
	delRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		nil, tok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Get returns 404 after deletion.
	getAfterDel := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		nil, tok), app.routes())
	assert.Equal(t, getAfterDel.StatusCode, http.StatusNotFound)
}

func TestCreateEnvironmentValidation(t *testing.T) {
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "envval@example.com", "Env Val", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Val WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	// Missing name returns 422.
	badRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"description": "no name"}, tok), app.routes())
	assert.Equal(t, badRes.StatusCode, http.StatusUnprocessableEntity)
}
