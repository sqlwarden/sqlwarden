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

	var roles []map[string]any
	if err := json.Unmarshal(rolesRes.BodyBytes, &roles); err != nil {
		t.Fatal(err)
	}

	var roleID int64
	for _, role := range roles {
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

	var envs []map[string]any
	if err := json.Unmarshal(listRes.BodyBytes, &envs); err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 accessible environment, got %d", len(envs))
	}
	if int64(envs[0]["id"].(float64)) != envAID {
		t.Fatalf("expected env A (id=%d), got id=%v", envAID, envs[0]["id"])
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
