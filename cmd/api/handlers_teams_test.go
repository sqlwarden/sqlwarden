package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestCreateAndListTeams(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "team-owner@example.com", "Team Owner", "securepass99")

	// Create a team.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "backend",
		"name": "Backend Team",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)

	// List teams.
	req2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusOK)

	var teams []map[string]any
	err := json.Unmarshal(res2.BodyBytes, &teams)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(teams), 1)
	assert.Equal(t, teams[0]["slug"].(string), "backend")
}

func TestListTeams_SupportsSearchAndSort(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "team-list-owner"), "Team Owner", "securepass99")

	for _, team := range []map[string]any{
		{"slug": "alpha", "name": "Alpha Team"},
		{"slug": "zeta", "name": "Zeta Team"},
	} {
		res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", team, tok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusCreated)
	}

	res := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams?q=ze&sort=name&order=desc", tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var teams []map[string]any
	decodeJSONResponse(t, res.BodyBytes, &teams)
	assert.Equal(t, len(teams), 1)
	assert.Equal(t, teams[0]["name"], "Zeta Team")
}

func TestGetAndDeleteTeam(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "team-crud@example.com", "Team CRUD", "securepass99")

	// Create a team.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "frontend",
		"name": "Frontend Team",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)

	// Get the team.
	req2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/frontend", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusOK)
	assert.Equal(t, res2.BodyFields["name"].(string), "Frontend Team")

	// Delete the team.
	req3 := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/teams/frontend", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	res3 := send(t, req3, app.routes())
	assert.Equal(t, res3.StatusCode, http.StatusNoContent)

	// Get returns 404 after deletion.
	req4 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/frontend", nil)
	req4.Header.Set("Authorization", "Bearer "+tok)
	res4 := send(t, req4, app.routes())
	assert.Equal(t, res4.StatusCode, http.StatusNotFound)
}

func TestTeamMemberManagement(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	ownerID, ownerTok, slug := registerAndLogin(t, app, "tm-owner@example.com", "TM Owner", "securepass99")

	// Register a second user.
	memberID, _, _ := registerAndLogin(t, app, "tm-member@example.com", "TM Member", "securepass99")

	// Add second user to the org.
	addOrgReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "tm-member@example.com",
		"role":  "member",
	})
	addOrgReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addOrgRes := send(t, addOrgReq, app.routes())
	assert.Equal(t, addOrgRes.StatusCode, http.StatusNoContent)

	// Create a team.
	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "devs",
		"name": "Developers",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)

	memberIDInt, _ := strconv.ParseInt(memberID, 10, 64)

	// Add member to team.
	addReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams/devs/members", map[string]any{
		"account_id": memberIDInt,
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// List team members.
	listReq := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/devs/members", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerTok)
	listRes := send(t, listReq, app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var members []map[string]any
	err := json.Unmarshal(listRes.BodyBytes, &members)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(members), 1)
	assert.Equal(t, fmt.Sprintf("%v", members[0]["account_id"]), memberID)

	// Remove member from team.
	removeReq := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/teams/devs/members/"+memberID, nil)
	removeReq.Header.Set("Authorization", "Bearer "+ownerTok)
	removeRes := send(t, removeReq, app.routes())
	assert.Equal(t, removeRes.StatusCode, http.StatusNoContent)

	// Verify the member is removed.
	listReq2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/devs/members", nil)
	listReq2.Header.Set("Authorization", "Bearer "+ownerTok)
	listRes2 := send(t, listReq2, app.routes())
	assert.Equal(t, listRes2.StatusCode, http.StatusOK)

	var members2 []map[string]any
	err = json.Unmarshal(listRes2.BodyBytes, &members2)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(members2), 0)

	// Suppress unused variable warnings.
	_ = ownerID
}

func TestCreateTeamValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "team-val@example.com", "Team Val", "securepass99")

	// Empty slug should fail.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "",
		"name": "Some Team",
	})
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	// Invalid slug characters should fail.
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "INVALID SLUG!",
		"name": "Some Team",
	})
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestTeamNotFoundBranches(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	ownerID, ownerTok, slug := registerAndLogin(t, app, "team-missing@example.com", "Team Missing", "securepass99")

	getRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/missing", nil, ownerTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusNotFound)

	listMembersRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/teams/missing/members", nil, ownerTok), app.routes())
	assert.Equal(t, listMembersRes.StatusCode, http.StatusNotFound)

	addMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams/missing/members", map[string]any{
		"account_id": 1,
	}, ownerTok), app.routes())
	assert.Equal(t, addMemberRes.StatusCode, http.StatusNotFound)

	removeMemberRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/teams/missing/members/1", nil, ownerTok), app.routes())
	assert.Equal(t, removeMemberRes.StatusCode, http.StatusNotFound)

	deleteRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/teams/missing", nil, ownerTok), app.routes())
	assert.Equal(t, deleteRes.StatusCode, http.StatusNotFound)

	_ = ownerID
}

func TestTeamMemberValidationAndInvalidID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, ownerTok, slug := registerAndLogin(t, app, "team-validation@example.com", "Team Validation", "securepass99")

	createReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams", map[string]any{
		"slug": "ops",
		"name": "Operations",
	})
	createReq.Header.Set("Authorization", "Bearer "+ownerTok)
	createRes := send(t, createReq, app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)

	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams/ops/members", map[string]any{}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusUnprocessableEntity)

	removeRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/teams/ops/members/not-a-number", nil, ownerTok), app.routes())
	assert.Equal(t, removeRes.StatusCode, http.StatusNotFound)
}
