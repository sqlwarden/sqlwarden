package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// setupPolicyTest creates an org, a workspace, and a member account.
// Returns ownerTok, memberTok, orgSlug, wsID, memberIDInt.
func setupPolicyTest(t *testing.T, app *application, suffix string) (ownerTok, memberTok, orgSlug string, wsID string, memberIDInt int64) {
	t.Helper()

	owner, ownerToken, org := seedOrgOwner(t, app, "access-owner-"+suffix+"@example.com", "Owner", "Owner's Org")
	member, memberToken := seedAccountWithToken(t, app, "access-member-"+suffix+"@example.com", "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	ws := seedWorkspaceForAccount(t, app, org, owner, "AccessWS", "")

	ownerTok = ownerToken
	memberTok = memberToken
	orgSlug = org.Slug
	wsID = strconv.FormatInt(ws.ID, 10)
	memberIDInt = member.ID

	return
}

func policiesURL(orgSlug, wsID string) string {
	return "/api/v1/orgs/" + orgSlug + "/workspaces/" + wsID + "/policies"
}

func createWorkspaceEnvironment(t *testing.T, app *application, ownerTok, orgSlug, wsID, name string) int64 {
	t.Helper()

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": name}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	return int64(res.BodyFields["id"].(float64))
}

func createWorkspaceConnection(t *testing.T, app *application, ownerTok, orgSlug, wsID, name string, environmentID *int64) int64 {
	t.Helper()

	body := map[string]any{
		"name":   name,
		"driver": "postgres",
		"dsn":    "postgres://localhost/testdb",
	}
	if environmentID != nil {
		body["environment_id"] = *environmentID
	}

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		body, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	return int64(res.BodyFields["id"].(float64))
}

func TestGrantWorkspacePermissionBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-perm")

	// Member cannot update workspace yet.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Hacked"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)

	// Owner grants ws:write directly to member (no role).
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Member can now update workspace.
	patchRes2 := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Updated"}, memberTok), app.routes())
	assert.Equal(t, patchRes2.StatusCode, http.StatusNoContent)
}

func TestGetWorkspaceRequiresAccessibleBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-get")

	getBefore := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getBefore.StatusCode, http.StatusNotFound)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getAfter := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getAfter.StatusCode, http.StatusOK)
}

func TestGetWorkspaceAccessibleViaOrgPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-org-ancestor")

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/policies",
		map[string]any{
			"permissions":  []string{"org:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetWorkspaceAccessibleViaEnvironmentPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-env-propagated")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"env:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetWorkspaceAccessibleViaConnectionPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-conn-propagated")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestWorkspaceVisibilityFromConnectionPermissionDoesNotGrantWrite(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-conn-visible-only")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)

	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Nope"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)
}

func TestRevokeWorkspacePermissionBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-revoke")

	// Grant ws:write.
	send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())

	// List bindings to get the permission binding ID.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	items := body["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected at least one permission binding")
	}
	pbID := fmt.Sprintf("%v", items[0].(map[string]any)["binding_id"])

	// Revoke it with ?kind=permission.
	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		policiesURL(orgSlug, wsID)+"/"+pbID+"?kind=permission",
		nil, ownerTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	// Member can no longer update workspace.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Hacked"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)
}

func TestRevokeWorkspaceRoleBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-role-revoke")

	rolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/roles", nil, ownerTok), app.routes())
	assert.Equal(t, rolesRes.StatusCode, http.StatusOK)

	var rolePayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rolesRes.BodyBytes, &rolePayload); err != nil {
		t.Fatal(err)
	}

	var roleID int64
	for _, role := range rolePayload.Items {
		if role["name"] == "ws:member" {
			roleID = int64(role["id"].(float64))
			break
		}
	}
	if roleID == 0 {
		t.Fatal("expected ws:member role to exist")
	}

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"role_id":      roleID,
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	items := body["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected role binding to be listed")
	}
	rbID := fmt.Sprintf("%v", items[0].(map[string]any)["binding_id"])

	memberListRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/roles", nil, memberTok), app.routes())
	assert.Equal(t, memberListRes.StatusCode, http.StatusForbidden)

	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		policiesURL(orgSlug, wsID)+"/"+rbID,
		nil, ownerTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	memberListRes = send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/roles", nil, memberTok), app.routes())
	assert.Equal(t, memberListRes.StatusCode, http.StatusForbidden)
}

func TestRevokeWorkspacePolicyCrossWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, _, orgSlug, ws1ID, memberIDInt := setupPolicyTest(t, app, "ws-cross-revoke")

	ws2Res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Other WS"}, ownerTok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, ws2ID),
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, ws2ID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbID := fmt.Sprintf("%v", body["items"].([]any)[0].(map[string]any)["binding_id"])

	revokeRes := send(t, newAuthRequest(t, http.MethodDelete,
		policiesURL(orgSlug, ws1ID)+"/"+pbID+"?kind=permission",
		nil, ownerTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNotFound)
}

func TestGrantPolicyValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-val")

	// Both role_id and permission set → 422.
	res := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"role_id":      1,
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	// Neither role_id nor permission → 422.
	res2 := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)

	// Invalid subject_type → 422.
	res3 := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "user",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res3.StatusCode, http.StatusUnprocessableEntity)

	// Invalid resource_type → 422.
	res4 := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"ws:write"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "bucket",
		}, ownerTok), app.routes())
	assert.Equal(t, res4.StatusCode, http.StatusUnprocessableEntity)

	// resource_type=connection without resource_id → 422.
	res5 := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:execute"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
		}, ownerTok), app.routes())
	assert.Equal(t, res5.StatusCode, http.StatusUnprocessableEntity)
}

func TestGrantConnectionPolicyBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-perm")

	// Create a connection.
	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "TestConn",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connIDFloat := connRes.BodyFields["id"].(float64)
	connID := int64(connIDFloat)

	// Grant conn:execute to member via the consolidated /policies endpoint.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:execute"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Binding appears in the workspace policies list.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbs := body["items"].([]any)
	found := false
	for _, pb := range pbs {
		b := pb.(map[string]any)
		if b["resource_type"] == "connection" && b["binding_kind"] == "permission" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a connection-scoped permission binding in the list")
	}

	_ = memberTok
}

func TestGrantConnectionPolicyWrongWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-wrongws")

	// Create a second workspace.
	ws2Res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "OtherWS"}, ownerTok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	// Create a connection in workspace 2.
	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws2ID+"/connections",
		map[string]any{
			"name":   "OtherConn",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connIDFloat := connRes.BodyFields["id"].(float64)
	connID := int64(connIDFloat)

	// Try to grant on workspace 1's /policies endpoint using conn from workspace 2 → 404.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:execute"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNotFound)
}

func TestGetConnectionRequiresAccessibleBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-get")

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "Primary DB",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	getBefore := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID, nil, memberTok), app.routes())
	assert.Equal(t, getBefore.StatusCode, http.StatusNotFound)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:metadata"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connRes.BodyFields["id"],
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getAfter := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID, nil, memberTok), app.routes())
	assert.Equal(t, getAfter.StatusCode, http.StatusOK)
}

func TestGetConnectionAccessibleViaWorkspacePermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-ws-ancestor")

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "Workspace Scoped DB",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetConnectionAccessibleViaEnvironmentPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-env-ancestor")

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := int64(envRes.BodyFields["id"].(float64))

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":           "Env Scoped DB",
			"driver":         "postgres",
			"dsn":            "postgres://localhost/testdb",
			"environment_id": envID,
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"env:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetConnectionAccessibleViaOrgPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "conn-org-ancestor")

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "Org Scoped DB",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/policies",
		map[string]any{
			"permissions":  []string{"org:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestOrgConnectionRoleGrantsDiscoveryAcrossMultipleBranches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, ws1ID, memberIDInt := setupPolicyTest(t, app, "org-conn-discovery")

	ws2Res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Second WS"}, ownerTok), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)
	ws2ID := fmt.Sprintf("%v", ws2Res.BodyFields["id"])

	envA1ID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, ws1ID, "env-a1")
	envA2ID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, ws1ID, "env-a2")
	envB1ID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, ws2ID, "env-b1")

	_ = createWorkspaceConnection(t, app, ownerTok, orgSlug, ws1ID, "conn-a1", &envA1ID)
	_ = createWorkspaceConnection(t, app, ownerTok, orgSlug, ws1ID, "conn-a2", &envA2ID)
	_ = createWorkspaceConnection(t, app, ownerTok, orgSlug, ws2ID, "conn-b1", &envB1ID)
	_ = createWorkspaceConnection(t, app, ownerTok, orgSlug, ws2ID, "conn-b2", nil)

	rolesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/roles", nil, ownerTok), app.routes())
	assert.Equal(t, rolesRes.StatusCode, http.StatusOK)

	var rolePayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rolesRes.BodyBytes, &rolePayload); err != nil {
		t.Fatal(err)
	}

	var roleID int64
	for _, role := range rolePayload.Items {
		if role["name"] == "org-conn-reader" {
			roleID = int64(role["id"].(float64))
			break
		}
	}
	if roleID == 0 {
		createRoleRes := send(t, newAuthRequest(t, http.MethodPost,
			"/api/v1/orgs/"+orgSlug+"/roles",
			map[string]any{
				"name":        "org-conn-reader",
				"description": "Org-scoped connection reader",
				"scope_type":  "org",
				"permissions": []string{"conn:read"},
			}, ownerTok), app.routes())
		assert.Equal(t, createRoleRes.StatusCode, http.StatusCreated)
		roleID = int64(createRoleRes.BodyFields["id"].(float64))
	}

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	workspacesRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces", nil, memberTok), app.routes())
	assert.Equal(t, workspacesRes.StatusCode, http.StatusOK)
	var workspacesPayload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(workspacesRes.BodyBytes, &workspacesPayload); err != nil {
		t.Fatal(err)
	}
	if workspacesPayload.Total != 2 {
		t.Fatalf("expected both workspaces visible via org conn role, got %+v", workspacesPayload.Items)
	}

	envsWs1Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws1ID+"/environments", nil, memberTok), app.routes())
	assert.Equal(t, envsWs1Res.StatusCode, http.StatusOK)
	var envsWs1Payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(envsWs1Res.BodyBytes, &envsWs1Payload); err != nil {
		t.Fatal(err)
	}
	if envsWs1Payload.Total != 2 {
		t.Fatalf("expected both ws1 environments visible via org conn role, got %+v", envsWs1Payload.Items)
	}

	envsWs2Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws2ID+"/environments", nil, memberTok), app.routes())
	assert.Equal(t, envsWs2Res.StatusCode, http.StatusOK)
	var envsWs2Payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(envsWs2Res.BodyBytes, &envsWs2Payload); err != nil {
		t.Fatal(err)
	}
	if envsWs2Payload.Total != 1 {
		t.Fatalf("expected only tagged ws2 environment visible, got %+v", envsWs2Payload.Items)
	}

	connsWs1Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws1ID+"/connections", nil, memberTok), app.routes())
	assert.Equal(t, connsWs1Res.StatusCode, http.StatusOK)
	var connsWs1Payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(connsWs1Res.BodyBytes, &connsWs1Payload); err != nil {
		t.Fatal(err)
	}
	if connsWs1Payload.Total != 2 {
		t.Fatalf("expected both ws1 connections visible via org conn role, got %+v", connsWs1Payload.Items)
	}

	connsWs2Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws2ID+"/connections", nil, memberTok), app.routes())
	assert.Equal(t, connsWs2Res.StatusCode, http.StatusOK)
	var connsWs2Payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(connsWs2Res.BodyBytes, &connsWs2Payload); err != nil {
		t.Fatal(err)
	}
	if connsWs2Payload.Total != 2 {
		t.Fatalf("expected both ws2 connections visible via org conn role, got %+v", connsWs2Payload.Items)
	}

	wsPatchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws1ID,
		map[string]any{"name": "Nope"}, memberTok), app.routes())
	assert.Equal(t, wsPatchRes.StatusCode, http.StatusForbidden)

	envPatchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+ws1ID+"/environments/"+strconv.FormatInt(envA1ID, 10),
		map[string]any{"name": "Nope"}, memberTok), app.routes())
	assert.Equal(t, envPatchRes.StatusCode, http.StatusForbidden)
}

func TestGrantEnvironmentPolicyBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-perm")

	// Create environment.
	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envIDFloat := envRes.BodyFields["id"].(float64)
	envID := int64(envIDFloat)

	// Grant env:deploy to member via the consolidated /policies endpoint.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"env:deploy"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Binding appears in the workspace policies list.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbs := body["items"].([]any)
	found := false
	for _, pb := range pbs {
		b := pb.(map[string]any)
		if b["resource_type"] == "environment" && b["binding_kind"] == "permission" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected an environment-scoped permission binding in the list")
	}
}

func TestGetEnvironmentRequiresAccessibleBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-get")

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	getBefore := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID, nil, memberTok), app.routes())
	assert.Equal(t, getBefore.StatusCode, http.StatusNotFound)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"env:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envRes.BodyFields["id"],
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getAfter := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID, nil, memberTok), app.routes())
	assert.Equal(t, getAfter.StatusCode, http.StatusOK)
}

func TestGetEnvironmentAccessibleViaWorkspacePermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-ws-ancestor")

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetEnvironmentAccessibleViaOrgPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-org-ancestor")

	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/policies",
		map[string]any{
			"permissions":  []string{"org:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestGetEnvironmentAccessibleViaConnectionPermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-conn-propagated")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+strconv.FormatInt(envID, 10), nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
}

func TestEnvironmentVisibilityFromConnectionPermissionDoesNotGrantWrite(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-conn-visible-only")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+strconv.FormatInt(envID, 10), nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)

	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+strconv.FormatInt(envID, 10),
		map[string]any{"name": "Nope"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)
}

func TestListWorkspacesIncludesWorkspaceFromConnectionAccess(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "ws-list-conn-propagated")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces", nil, memberTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 || fmt.Sprintf("%v", payload.Items[0]["id"]) != wsID {
		t.Fatalf("expected propagated workspace visibility for %s, got %+v", wsID, payload.Items)
	}
}

func TestListEnvironmentsIncludesEnvironmentFromConnectionAccess(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-list-conn-propagated")

	envID := createWorkspaceEnvironment(t, app, ownerTok, orgSlug, wsID, "staging")
	connID := createWorkspaceConnection(t, app, ownerTok, orgSlug, wsID, "primary-db", &envID)

	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":   []string{"conn:read"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments", nil, memberTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 || fmt.Sprintf("%v", payload.Items[0]["id"]) != strconv.FormatInt(envID, 10) {
		t.Fatalf("expected propagated environment visibility for %d, got %+v", envID, payload.Items)
	}
}

func TestListPoliciesShowsAllResourceTypes(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "list-all")

	// Create environment and connection.
	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "prod"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envIDFloat := envRes.BodyFields["id"].(float64)
	envID := int64(envIDFloat)

	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "ProdDB",
			"driver": "postgres",
			"dsn":    "postgres://localhost/prod",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connIDFloat := connRes.BodyFields["id"].(float64)
	connID := int64(connIDFloat)

	// Grant bindings at workspace, environment, and connection scope.
	for _, body := range []map[string]any{
		{
			"permissions":  []string{"ws:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		},
		{
			"permissions":   []string{"env:deploy"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envID,
		},
		{
			"permissions":   []string{"conn:execute"},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "connection",
			"resource_id":   connID,
		},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, policiesURL(orgSlug, wsID), body, ownerTok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNoContent)
	}

	// Single list call must return all three bindings.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var resp map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &resp); err != nil {
		t.Fatal(err)
	}
	pbs := resp["items"].([]any)
	permissionCount := 0

	types := map[string]bool{}
	for _, pb := range pbs {
		b := pb.(map[string]any)
		if b["binding_kind"] == "permission" {
			permissionCount++
			types[b["resource_type"].(string)] = true
		}
	}
	assert.Equal(t, permissionCount, 3)
	if !types["workspace"] || !types["environment"] || !types["connection"] {
		t.Fatalf("expected bindings for all three resource types, got: %v", types)
	}
}

// TestEnvScopedRoleGrantsConnectionListAndConnect verifies the full HTTP flow for the
// "team per environment" use case:
//   - member with only env-A-scoped permissions can list connections in env A
//   - member can connect and query via those connections
//   - member is denied connect on connections in env B
//   - member does not see env B connections in the list
func TestEnvScopedRoleGrantsConnectionListAndConnect(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-conn-e2e")
	baseURL := "/api/v1/orgs/" + orgSlug + "/workspaces/" + wsID

	// Create two environments.
	envARes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/environments",
		map[string]any{"name": "env-a"}, ownerTok), app.routes())
	assert.Equal(t, envARes.StatusCode, http.StatusCreated)
	envAID := int64(envARes.BodyFields["id"].(float64))

	envBRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/environments",
		map[string]any{"name": "env-b"}, ownerTok), app.routes())
	assert.Equal(t, envBRes.StatusCode, http.StatusCreated)
	envBID := int64(envBRes.BodyFields["id"].(float64))

	// Create one connection tagged to env A, one tagged to env B, one untagged.
	connARes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/connections", map[string]any{
		"name": "conn-a", "driver": "sqlite", "dsn": "file::memory:?cache=shared",
		"environment_id": envAID,
	}, ownerTok), app.routes())
	assert.Equal(t, connARes.StatusCode, http.StatusCreated)
	connAID := fmt.Sprintf("%v", connARes.BodyFields["id"])

	connBRes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/connections", map[string]any{
		"name": "conn-b", "driver": "sqlite", "dsn": "file::memory:?cache=shared",
		"environment_id": envBID,
	}, ownerTok), app.routes())
	assert.Equal(t, connBRes.StatusCode, http.StatusCreated)
	connBID := fmt.Sprintf("%v", connBRes.BodyFields["id"])

	send(t, newAuthRequest(t, http.MethodPost, baseURL+"/connections", map[string]any{
		"name": "conn-untagged", "driver": "sqlite", "dsn": "file::memory:?cache=shared",
	}, ownerTok), app.routes())

	// Grant env-A-scoped permissions to the member only.
	for _, perm := range []string{"conn:execute", "conn:read", "conn:metadata", "query:execute"} {
		res := send(t, newAuthRequest(t, http.MethodPost, policiesURL(orgSlug, wsID), map[string]any{
			"permissions":   []string{perm},
			"subject_type":  "account",
			"subject_id":    memberIDInt,
			"resource_type": "environment",
			"resource_id":   envAID,
		}, ownerTok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNoContent)
	}

	// Member lists connections — only conn A should appear.
	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL+"/connections", nil, memberTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 accessible connection, got %d: %v", len(payload.Items), payload.Items)
	}
	if fmt.Sprintf("%v", payload.Items[0]["id"]) != connAID {
		t.Fatalf("expected conn A (%s) in list, got id=%v", connAID, payload.Items[0]["id"])
	}

	// Member can connect to conn A.
	connectRes := send(t, newAuthRequest(t, http.MethodPost,
		baseURL+"/connections/"+connAID+"/connect", nil, memberTok), app.routes())
	assert.Equal(t, connectRes.StatusCode, http.StatusOK)
	sessionID := connectRes.BodyFields["session_id"].(string)

	// Member can query via conn A.
	queryReq := newAuthRequest(t, http.MethodPost,
		baseURL+"/connections/"+connAID+"/query",
		map[string]any{"sql": "SELECT 1"}, memberTok)
	queryReq.Header.Set("X-Warden-Session", sessionID)
	assert.Equal(t, send(t, queryReq, app.routes()).StatusCode, http.StatusOK)

	// Member cannot connect to conn B (env B — no binding there).
	connectBRes := send(t, newAuthRequest(t, http.MethodPost,
		baseURL+"/connections/"+connBID+"/connect", nil, memberTok), app.routes())
	assert.Equal(t, connectBRes.StatusCode, http.StatusForbidden)
}

// TestEnvScopedRoleFiltersEnvironmentList verifies that listEnvironments returns only
// the environments the member has a binding on (env-A-only binding hides env B).
func TestEnvScopedRoleFiltersEnvironmentList(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "env-list-filter")
	baseURL := "/api/v1/orgs/" + orgSlug + "/workspaces/" + wsID

	envARes := send(t, newAuthRequest(t, http.MethodPost, baseURL+"/environments",
		map[string]any{"name": "env-a"}, ownerTok), app.routes())
	assert.Equal(t, envARes.StatusCode, http.StatusCreated)
	envAID := int64(envARes.BodyFields["id"].(float64))

	send(t, newAuthRequest(t, http.MethodPost, baseURL+"/environments",
		map[string]any{"name": "env-b"}, ownerTok), app.routes())

	// Grant env:read on env A only.
	grantRes := send(t, newAuthRequest(t, http.MethodPost, policiesURL(orgSlug, wsID), map[string]any{
		"permissions":   []string{"env:read"},
		"subject_type":  "account",
		"subject_id":    memberIDInt,
		"resource_type": "environment",
		"resource_id":   envAID,
	}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, baseURL+"/environments", nil, memberTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var envs struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &envs); err != nil {
		t.Fatal(err)
	}
	if envs.Total != 1 {
		t.Fatalf("expected 1 accessible environment, got %d", envs.Total)
	}
	if int64(envs.Items[0]["id"].(float64)) != envAID {
		t.Fatalf("expected env A (id=%d), got id=%v", envAID, envs.Items[0]["id"])
	}
}

// TestGrantMultiplePermissions verifies that a single POST /policies with multiple
// permissions creates one binding per permission and all are enforced.
func TestGrantMultiplePermissions(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupPolicyTest(t, app, "multi-perm")

	// Member has no permissions yet.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Attempt"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)

	// Grant ws:write AND ws:read in a single request.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		policiesURL(orgSlug, wsID),
		map[string]any{
			"permissions":  []string{"ws:write", "ws:read"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Both permissions are now enforced: member can update workspace.
	patchRes2 := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Updated"}, memberTok), app.routes())
	assert.Equal(t, patchRes2.StatusCode, http.StatusNoContent)

	// Two permission bindings should appear in the list.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		policiesURL(orgSlug, wsID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	items := body["items"].([]any)
	permissionCount := 0
	for _, item := range items {
		if item.(map[string]any)["binding_kind"] == "permission" {
			permissionCount++
		}
	}
	assert.Equal(t, permissionCount, 2)
}
