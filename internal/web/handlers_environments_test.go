package web

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestEnvironmentLifecycle(t *testing.T) {
	t.Parallel()
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

	var listPayload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &listPayload)
	assert.Equal(t, listPayload.Total, 2)

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
	t.Parallel()
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

func TestCreateEnvironmentDuplicateNameReturns422(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "envdup@example.com", "Env Dup", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Dup WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	create1 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, create1.StatusCode, http.StatusCreated)

	create2 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, create2.StatusCode, http.StatusUnprocessableEntity)
}

func TestListEnvironments_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "env-list-owner"), "Env Owner", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Env List WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	for _, name := range []string{"staging", "dev", "prod"} {
		res := send(t, newAuthRequest(t, http.MethodPost,
			"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
			map[string]any{"name": name}, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newOrgRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments?q=pro&name=prod&sort=name&order=asc&page=1&page_size=1", tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "prod")
}

func TestUpdateEnvironmentValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "env-update-val@example.com", "Env Update Val", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Update Val WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		map[string]any{"description": "missing name"}, tok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusUnprocessableEntity)
}

func TestUpdateEnvironment_RejectsWorkspaceChange(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "env-immutable-owner"), "Env Immutable", "securepass99")

	wsRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces",
		map[string]any{"name": "Immutable WS"}, tok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, tok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	res := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+slug+"/workspaces/"+wsID+"/environments/"+envID,
		map[string]any{"name": "prod", "workspace_id": 9999}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "workspace_id")
}

func TestEnvironmentMutationsRequireScopedPermissions(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberID := setupPolicyTest(t, app, "env-mutation-perms")
	org, workspaceID := policyScope(t, app, orgSlug, wsID)
	baseURL := "/api/v1/orgs/" + orgSlug + "/workspaces/" + wsID + "/environments"

	createDenied := send(t, newAuthRequest(t, http.MethodPost, baseURL,
		map[string]any{"name": "qa"}, memberTok), app.routes())
	assert.Equal(t, createDenied.StatusCode, http.StatusForbidden)

	createRoleID := createRoleForTest(t, app, org.ID, &workspaceID, "workspace", "env:create")
	grantCreate := grantWorkspacePolicyRole(t, app, ownerTok, orgSlug, wsID, createRoleID, "account", memberID, "workspace", 0)
	assert.Equal(t, grantCreate.StatusCode, http.StatusNoContent)

	createAllowed := send(t, newAuthRequest(t, http.MethodPost, baseURL,
		map[string]any{"name": "qa"}, memberTok), app.routes())
	assert.Equal(t, createAllowed.StatusCode, http.StatusCreated)
	envID := int64(createAllowed.BodyFields["id"].(float64))
	envURL := baseURL + "/" + strconv.FormatInt(envID, 10)

	updateDenied := send(t, newAuthRequest(t, http.MethodPatch, envURL,
		map[string]any{"name": "qa-renamed"}, memberTok), app.routes())
	assert.Equal(t, updateDenied.StatusCode, http.StatusForbidden)

	updateRoleID := createRoleForTest(t, app, org.ID, &workspaceID, "environment", "env:write")
	grantUpdate := grantWorkspacePolicyRole(t, app, ownerTok, orgSlug, wsID, updateRoleID, "account", memberID, "environment", envID)
	assert.Equal(t, grantUpdate.StatusCode, http.StatusNoContent)

	updateAllowed := send(t, newAuthRequest(t, http.MethodPatch, envURL,
		map[string]any{"name": "qa-renamed"}, memberTok), app.routes())
	assert.Equal(t, updateAllowed.StatusCode, http.StatusNoContent)

	deleteDenied := send(t, newAuthRequest(t, http.MethodDelete, envURL, nil, memberTok), app.routes())
	assert.Equal(t, deleteDenied.StatusCode, http.StatusForbidden)

	deleteRoleID := createRoleForTest(t, app, org.ID, &workspaceID, "environment", "env:delete")
	grantDelete := grantWorkspacePolicyRole(t, app, ownerTok, orgSlug, wsID, deleteRoleID, "account", memberID, "environment", envID)
	assert.Equal(t, grantDelete.StatusCode, http.StatusNoContent)

	deleteAllowed := send(t, newAuthRequest(t, http.MethodDelete, envURL, nil, memberTok), app.routes())
	assert.Equal(t, deleteAllowed.StatusCode, http.StatusNoContent)
}
