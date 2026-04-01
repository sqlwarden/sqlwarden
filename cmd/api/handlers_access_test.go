package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// setupAccessTest creates an org, a workspace, and a member account.
// Returns ownerTok, memberTok, orgSlug, wsID, memberIDInt.
func setupAccessTest(t *testing.T, app *application, suffix string) (ownerTok, memberTok, orgSlug string, wsID string, memberIDInt int64) {
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

func TestGrantWorkspacePermissionBinding(t *testing.T) {
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "ws-perm")

	// Member cannot update workspace yet.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Hacked"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)

	// Owner grants ws:write directly to member (no role).
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
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
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "ws-revoke")

	// Grant ws:write.
	send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())

	// List bindings to get the permission binding ID.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access", nil, ownerTok), app.routes())
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
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access/"+pbID+"?kind=permission",
		nil, ownerTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	// Member can no longer update workspace.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Hacked"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)
}

func TestGrantAccessValidation(t *testing.T) {
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "ws-val")

	// Both role_id and permission set → 422.
	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
		map[string]any{
			"role_id":      1,
			"permissions":  []string{"ws:write"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	// Neither role_id nor permission → 422.
	res2 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
		map[string]any{
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)

	// Invalid subject_type → 422.
	res3 := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
		map[string]any{
			"permissions":  []string{"ws:write"},
			"subject_type": "user",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, res3.StatusCode, http.StatusUnprocessableEntity)
}

func TestGrantConnectionPermissionBinding(t *testing.T) {
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "conn-perm")

	// Create a connection.
	connRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections",
		map[string]any{
			"name":   "TestConn",
			"driver": "postgres",
			"dsn":    "postgres://localhost/testdb",
		}, ownerTok), app.routes())
	assert.Equal(t, connRes.StatusCode, http.StatusCreated)
	connID := fmt.Sprintf("%v", connRes.BodyFields["id"])

	// Grant conn:execute directly (no role) to member on the connection.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID+"/access",
		map[string]any{
			"permissions":  []string{"conn:execute"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Verify binding appears in the list.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/connections/"+connID+"/access",
		nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbs := body["permission_bindings"].([]any)
	if len(pbs) == 0 {
		t.Fatal("expected permission_bindings to contain the granted binding")
	}

	_ = memberTok
}

func TestGrantEnvironmentPermissionBinding(t *testing.T) {
	app := newTestApp(t)
	ownerTok, _, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "env-perm")

	// Create environment.
	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	// Grant env:deploy directly (no role) to member.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID+"/access",
		map[string]any{
			"permissions":  []string{"env:deploy"},
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	// Verify it's listed.
	listRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID+"/access",
		nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbs := body["permission_bindings"].([]any)
	if len(pbs) == 0 {
		t.Fatal("expected permission_bindings to contain the granted binding")
	}
}

// TestGrantMultiplePermissions verifies that a single POST /access with multiple permissions
// creates one binding per permission and all are enforced.
func TestGrantMultiplePermissions(t *testing.T) {
	app := newTestApp(t)
	ownerTok, memberTok, orgSlug, wsID, memberIDInt := setupAccessTest(t, app, "multi-perm")

	// Member has no permissions yet.
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Attempt"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)

	// Grant ws:write AND ws:read in a single request.
	grantRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access",
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
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/access", nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	pbs := body["permission_bindings"].([]any)
	assert.Equal(t, len(pbs), 2)
}
