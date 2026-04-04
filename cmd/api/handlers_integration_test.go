package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

// TestFullWorkflow exercises the complete multi-step lifecycle:
// register → create org → workspace → environment → role → bind → permission check → cleanup.
func TestFullWorkflow(t *testing.T) {
	app := newTestApp(t)

	// ── Step 1: Register two users ───────────────────────────────────────────
	// Owner uses setup (first account = instance admin).
	ownerTok := setupInstance(t, app, "flow-owner@example.com", "Flow Owner", "securepass99")

	memberRes := registerTestUser(t, app, "flow-member@example.com", "Flow Member", "securepass99")
	assert.Equal(t, memberRes.StatusCode, http.StatusCreated)
	memberID := fmt.Sprintf("%v", memberRes.BodyFields["id"])

	// ── Step 3: Create org ───────────────────────────────────────────────────
	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs",
		map[string]any{"name": "Flow Corp"}, ownerTok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusCreated)
	orgSlug := orgRes.BodyFields["slug"].(string)

	// ── Step 4: Add member to org ────────────────────────────────────────────
	addMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/members",
		map[string]any{"email": "flow-member@example.com", "role": "member"}, ownerTok), app.routes())
	assert.Equal(t, addMemberRes.StatusCode, http.StatusNoContent)

	// ── Step 5: Create workspace ─────────────────────────────────────────────
	wsRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/workspaces",
		map[string]any{"name": "Production", "description": "Prod workspace"}, ownerTok), app.routes())
	assert.Equal(t, wsRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", wsRes.BodyFields["id"])

	// ── Step 6: Create environment inside workspace ──────────────────────────
	envRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		map[string]any{"name": "staging", "description": "Staging environment"}, ownerTok), app.routes())
	assert.Equal(t, envRes.StatusCode, http.StatusCreated)
	envID := fmt.Sprintf("%v", envRes.BodyFields["id"])

	// ── Step 7: Create a custom role ─────────────────────────────────────────
	roleRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/roles",
		map[string]any{
			"name":        "viewer",
			"scope_type":  "workspace",
			"permissions": []string{"ws:read", "env:read", "conn:metadata"},
		}, ownerTok), app.routes())
	assert.Equal(t, roleRes.StatusCode, http.StatusCreated)
	roleID := fmt.Sprintf("%v", roleRes.BodyFields["id"])

	// ── Step 8: Bind the custom role to the member at workspace scope ────────
	memberIDInt := 0
	fmt.Sscanf(memberID, "%d", &memberIDInt)
	roleIDInt := 0
	fmt.Sscanf(roleID, "%d", &roleIDInt)

	bindRes := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/policies",
		map[string]any{
			"role_id":      roleIDInt,
			"subject_type": "account",
			"subject_id":   memberIDInt,
		}, ownerTok), app.routes())
	assert.Equal(t, bindRes.StatusCode, http.StatusNoContent)

	// ── Step 9: Member logs in and verifies workspace access ─────────────────
	memberLoginRes := loginTestUser(t, app, "flow-member@example.com", "securepass99")
	assert.Equal(t, memberLoginRes.StatusCode, http.StatusOK)
	memberTok := extractAccessToken(t, memberLoginRes)

	// Member can GET workspace (has ws:read via viewer role).
	getWsRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getWsRes.StatusCode, http.StatusOK)
	assert.Equal(t, getWsRes.BodyFields["name"].(string), "Production")

	// Member cannot update workspace (requires ws:write, not in viewer role).
	patchRes := send(t, newAuthRequest(t, http.MethodPatch,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		map[string]any{"name": "Hacked"}, memberTok), app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusForbidden)

	// ── Step 10: List org members ────────────────────────────────────────────
	membersRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/members", nil, ownerTok), app.routes())
	assert.Equal(t, membersRes.StatusCode, http.StatusOK)

	var members struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(membersRes.BodyBytes, &members); err != nil {
		t.Fatal(err)
	}
	if members.Total < 2 {
		t.Fatalf("expected at least 2 members, got %d", members.Total)
	}

	// ── Step 11: List environments ───────────────────────────────────────────
	listEnvRes := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments",
		nil, ownerTok), app.routes())
	assert.Equal(t, listEnvRes.StatusCode, http.StatusOK)

	var envs struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listEnvRes.BodyBytes, &envs); err != nil {
		t.Fatal(err)
	}
	if envs.Total != 1 {
		t.Fatalf("expected 1 environment, got %d", envs.Total)
	}

	// ── Step 12: Delete environment ──────────────────────────────────────────
	delEnvRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID+"/environments/"+envID,
		nil, ownerTok), app.routes())
	assert.Equal(t, delEnvRes.StatusCode, http.StatusNoContent)

	// ── Step 13: Delete workspace ────────────────────────────────────────────
	delWsRes := send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID,
		nil, ownerTok), app.routes())
	assert.Equal(t, delWsRes.StatusCode, http.StatusNoContent)

	// Workspace is gone.
	getWsAfterDel := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+orgSlug+"/workspaces/"+wsID, nil, ownerTok), app.routes())
	assert.Equal(t, getWsAfterDel.StatusCode, http.StatusNotFound)
}

// TestRBACIsolation verifies that users in different orgs cannot access each other's resources.
func TestRBACIsolation(t *testing.T) {
	app := newTestApp(t)

	_, tok1, slug1 := registerAndLogin(t, app, "iso-owner1@example.com", "ISO Owner1", "securepass99")
	_, tok2, slug2 := registerAndLogin(t, app, "iso-owner2@example.com", "ISO Owner2", "securepass99")

	// Each owner creates a workspace in their own org.
	ws1Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug1+"/workspaces",
		map[string]any{"name": "WS1"}, tok1), app.routes())
	assert.Equal(t, ws1Res.StatusCode, http.StatusCreated)

	ws2Res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug2+"/workspaces",
		map[string]any{"name": "WS2"}, tok2), app.routes())
	assert.Equal(t, ws2Res.StatusCode, http.StatusCreated)

	// Owner1 cannot access org2's resources.
	getOrg2Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug2, nil, tok1), app.routes())
	assert.Equal(t, getOrg2Res.StatusCode, http.StatusForbidden)

	// Owner2 cannot access org1's resources.
	getOrg1Res := send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+slug1, nil, tok2), app.routes())
	assert.Equal(t, getOrg1Res.StatusCode, http.StatusForbidden)
}

// TestRefreshTokenRotation verifies the full refresh token rotation cycle.
func TestRefreshTokenRotation(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	registerTestUser(t, app, "rotate@example.com", "Rotate User", "securepass99")

	login1 := loginTestUser(t, app, "rotate@example.com", "securepass99")
	assert.Equal(t, login1.StatusCode, http.StatusOK)
	cookie1 := extractRefreshCookie(t, login1)

	// First refresh.
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	req2.AddCookie(cookie1)
	refresh1 := send(t, req2, app.routes())
	assert.Equal(t, refresh1.StatusCode, http.StatusOK)
	tok2 := extractAccessToken(t, refresh1)
	if tok2 == "" {
		t.Fatal("expected non-empty access token after first refresh")
	}
	cookie2 := extractRefreshCookie(t, refresh1)

	// Old cookie is revoked — reusing it should fail.
	req3 := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	req3.AddCookie(cookie1)
	reuse := send(t, req3, app.routes())
	assert.Equal(t, reuse.StatusCode, http.StatusUnauthorized)

	// New cookie from rotation still works.
	req4 := newTestRequest(t, http.MethodPost, "/api/v1/auth/refresh", nil)
	req4.AddCookie(cookie2)
	refresh2 := send(t, req4, app.routes())
	assert.Equal(t, refresh2.StatusCode, http.StatusOK)
}
