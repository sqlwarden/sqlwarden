package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// setupPolicyTest creates an org, a workspace, and a member account.
// Returns ownerTok, memberTok, orgSlug, wsID, memberIDInt.
func setupPolicyTest(t *testing.T, app *application, suffix string) (ownerTok, memberTok, orgSlug string, wsID string, memberIDInt int64) {
	t.Helper()

	_, ownerTok, orgSlug = registerAndLogin(t, app, "access-owner-"+suffix+"@example.com", "Owner", "securepass99")
	memberIDStr, memberTok2, _ := registerAndLogin(t, app, "access-member-"+suffix+"@example.com", "Member", "securepass99")
	memberTok = memberTok2

	fmt.Sscanf(memberIDStr, "%d", &memberIDInt)

	// Add member to org.
	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/members",
		map[string]any{"email": "access-member-" + suffix + "@example.com", "role": "member"}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Create workspace.
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "AccessWS"}, ownerTok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID = fmt.Sprintf("%v", wsRes.BodyFields["id"])

	return
}

func policiesURL(orgSlug, wsID string) string {
	return "/api/v1/orgs/" + orgSlug + "/workspaces/" + wsID + "/policies"
}

func TestGrantWorkspacePermissionBinding(t *testing.T) {
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

func TestRevokeWorkspacePermissionBinding(t *testing.T) {
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
	pbs := body["permission_bindings"].([]any)
	if len(pbs) == 0 {
		t.Fatal("expected at least one permission binding")
	}
	pbID := fmt.Sprintf("%v", pbs[0].(map[string]any)["id"])

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

func TestGrantPolicyValidation(t *testing.T) {
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
	pbs := body["permission_bindings"].([]any)
	found := false
	for _, pb := range pbs {
		b := pb.(map[string]any)
		if b["resource_type"] == "connection" {
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

func TestGrantEnvironmentPolicyBinding(t *testing.T) {
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
	pbs := body["permission_bindings"].([]any)
	found := false
	for _, pb := range pbs {
		b := pb.(map[string]any)
		if b["resource_type"] == "environment" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected an environment-scoped permission binding in the list")
	}
}

func TestListPoliciesShowsAllResourceTypes(t *testing.T) {
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
	pbs := resp["permission_bindings"].([]any)
	assert.Equal(t, len(pbs), 3)

	types := map[string]bool{}
	for _, pb := range pbs {
		b := pb.(map[string]any)
		types[b["resource_type"].(string)] = true
	}
	if !types["workspace"] || !types["environment"] || !types["connection"] {
		t.Fatalf("expected bindings for all three resource types, got: %v", types)
	}
}

// TestGrantMultiplePermissions verifies that a single POST /policies with multiple
// permissions creates one binding per permission and all are enforced.
func TestGrantMultiplePermissions(t *testing.T) {
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
	pbs := body["permission_bindings"].([]any)
	assert.Equal(t, len(pbs), 2)
}
