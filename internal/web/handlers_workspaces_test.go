package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func TestCreateAndListWorkspaces(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "ws-owner@example.com", "WS Owner", "securepass99")

	// Create a workspace.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name":        "Production",
		"description": "Prod workspace",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", res.BodyFields["id"])
	assert.Equal(t, int(res.BodyFields["environment_count"].(float64)), 1)
	assert.Equal(t, int(res.BodyFields["connection_count"].(float64)), 0)
	envID := createWorkspaceEnvironment(t, app, tok, slug, wsID, "Staging")
	createWorkspaceConnection(t, app, tok, slug, wsID, "Primary", &envID)

	// List workspaces.
	req2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	err := json.Unmarshal(res2.BodyBytes, &payload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 25)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"].(string), "Production")
	assert.Equal(t, int(payload.Items[0]["environment_count"].(float64)), 2)
	assert.Equal(t, int(payload.Items[0]["connection_count"].(float64)), 1)
}

func TestListWorkspaces_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "workspace-list-owner"), "Workspace Owner", "securepass99")

	for _, workspace := range []map[string]any{
		{"name": "Data Lake", "description": "Lake"},
		{"name": "Analytics", "description": "BI"},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", workspace, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces?q=data&name=Data%20Lake&sort=name&order=asc&page=1&page_size=1", tok), app.routes())
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
	assert.Equal(t, payload.Items[0]["name"], "Data Lake")
}

func TestListAndGetWorkspaces_AllowsWorkspacePolicyOnlyAccess(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "workspace-policy-owner"), "Workspace Policy Owner", "Workspace Policy Org")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "workspace-policy-member"), "Workspace Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	ws := seedWorkspaceForAccount(t, app, org, owner, "Policy Managed", "")
	hidden := seedWorkspaceForAccount(t, app, org, owner, "Hidden", "")
	_ = hidden
	wsID := strconv.FormatInt(ws.ID, 10)
	roleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", "policy:read")

	grantRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, wsID, roleID, "account", member.ID, "workspace", 0)
	assert.Equal(t, grantRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/workspaces", nil, memberTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, int64(payload.Items[0]["id"].(float64)), ws.ID)

	getRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/workspaces/"+wsID, nil, memberTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, int64(getRes.BodyFields["id"].(float64)), ws.ID)
}

func TestCreateOwnedWorkspace_RollsBackWorkspaceAndSeedOnFailure(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "workspace-rollback-owner"), "Workspace Rollback Owner", "Workspace Rollback Org")
	_ = owner

	_, err := app.createOwnedWorkspace(context.Background(), org.ID, 99999999, "Rollback Workspace", "")
	if err == nil {
		t.Fatal("expected workspace seed failure")
	}

	workspaces, err := app.db.ListWorkspacesPage(context.Background(), database.ListWorkspacesParams{
		OwnerType: "org",
		OwnerID:   org.ID,
		Name:      "Rollback Workspace",
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if workspaces.Total != 0 {
		t.Fatalf("expected workspace insert to roll back, got total %d", workspaces.Total)
	}

	var roleCount int
	if err := app.db.NewSelect().
		TableExpr("roles").
		ColumnExpr("COUNT(*)").
		Where("org_id = ? AND workspace_id IS NOT NULL", org.ID).
		Scan(context.Background(), &roleCount); err != nil {
		t.Fatal(err)
	}
	if roleCount != 0 {
		t.Fatalf("expected workspace roles to roll back, got %d", roleCount)
	}
}

func TestGetAndDeleteWorkspace(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "ws-crud@example.com", "WS CRUD", "securepass99")

	// Create.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "Staging",
	})
	createReq.Header.Set("Authorization", "Bearer "+tok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Get.
	getReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getRes := send(t, getReq, app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"].(string), "Staging")
	assert.Equal(t, int(getRes.BodyFields["environment_count"].(float64)), 1)
	assert.Equal(t, int(getRes.BodyFields["connection_count"].(float64)), 0)

	// Delete.
	delReq := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	delReq.Header.Set("Authorization", "Bearer "+tok)
	delRes := send(t, delReq, app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Get returns 404 after deletion.
	getReq2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, nil)
	getReq2.Header.Set("Authorization", "Bearer "+tok)
	getRes2 := send(t, getReq2, app.routes())
	assert.Equal(t, getRes2.StatusCode, http.StatusNotFound)
}

func TestWorkspaceUpdatePermission(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, ownerTok, slug := registerAndLogin(t, app, "ws-upd@example.com", "WS Upd", "securepass99")

	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/workspaces", map[string]any{
		"name": "UpdateMe",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	wsID := fmt.Sprintf("%v", createRes.BodyFields["id"])

	// Owner can update (owner has ws:write via builtin role).
	newName := "UpdatedName"
	patchReq := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/workspaces/"+wsID, map[string]any{
		"name": newName,
	})
	patchReq.Header.Set("Authorization", "Bearer "+ownerTok)
	patchRes := send(t, patchReq, app.routes())
	assert.Equal(t, patchRes.StatusCode, http.StatusNoContent)
}
