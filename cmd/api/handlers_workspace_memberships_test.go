package main

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
)

func workspaceMembersURL(orgSlug string, workspaceID int64) string {
	return "/api/v1/orgs/" + orgSlug + "/workspaces/" + strconv.FormatInt(workspaceID, 10) + "/users"
}

func workspaceTeamsURL(orgSlug string, workspaceID int64) string {
	return "/api/v1/orgs/" + orgSlug + "/workspaces/" + strconv.FormatInt(workspaceID, 10) + "/teams"
}

func workspacePoliciesURL(orgSlug string, workspaceID int64) string {
	return "/api/v1/orgs/" + orgSlug + "/workspaces/" + strconv.FormatInt(workspaceID, 10) + "/policies"
}

func containsInt64(values []int64, expected int64) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func listWorkspaceIDs(t *testing.T, app *application, token, orgSlug string) []int64 {
	t.Helper()

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgSlug+"/workspaces", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)

	ids := make([]int64, 0, len(payload.Items))
	for _, item := range payload.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func findWorkspacePolicyBindingID(t *testing.T, app *application, token, orgSlug string, workspaceID int64, subjectType, roleName string) int64 {
	t.Helper()

	res := send(t, newAuthRequest(t, http.MethodGet,
		workspacePoliciesURL(orgSlug, workspaceID)+"?subject_type="+subjectType+"&page=1&page_size=50",
		nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items []struct {
			BindingID   int64  `json:"binding_id"`
			SubjectType string `json:"subject_type"`
			RoleName    string `json:"role_name"`
		} `json:"items"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	for _, item := range payload.Items {
		if item.SubjectType == subjectType && item.RoleName == roleName {
			return item.BindingID
		}
	}
	t.Fatalf("workspace policy binding not found for subject_type=%s role_name=%s", subjectType, roleName)
	return 0
}

func TestWorkspaceMembersAPI_AddListRemoveAndVisibility(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-owner"), "Workspace Owner", "Workspace Members Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-member"), "Workspace Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	if containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("org member should not see workspace before workspace membership")
	}

	addRes := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID),
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	duplicateRes := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID),
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, duplicateRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, ws.ID)+"?q="+member.Email+"&sort=email&order=asc&page=1&page_size=10", nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var listPayload struct {
		Total int `json:"total"`
		Items []struct {
			AccountID int64  `json:"account_id"`
			Email     string `json:"email"`
			Name      string `json:"name"`
		} `json:"items"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &listPayload)
	assert.Equal(t, listPayload.Total, 1)
	assert.Equal(t, listPayload.Items[0].AccountID, member.ID)
	assert.Equal(t, listPayload.Items[0].Email, member.Email)

	if !containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("workspace member should see workspace through default workspace_members policy")
	}

	deleteRes := send(t, newAuthRequest(t, http.MethodDelete, workspaceMembersURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(member.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, deleteRes.StatusCode, http.StatusNoContent)
	deleteAgainRes := send(t, newAuthRequest(t, http.MethodDelete, workspaceMembersURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(member.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, deleteAgainRes.StatusCode, http.StatusNoContent)

	if containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("removed workspace member should no longer see workspace")
	}
}

func TestWorkspaceMembersAPI_RejectsNonOrgAndCrossOrgAccounts(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-cross-owner"), "Workspace Owner", "Workspace Members Cross Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	nonMember, _ := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-non-member"), "Non Member")
	otherOwner, _, _ := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-other-owner"), "Other Owner", "Other Workspace Members Org")

	for _, accountID := range []int64{nonMember.ID, otherOwner.ID} {
		res := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID),
			map[string]any{"account_id": accountID}, ownerTok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNotFound)
	}
}

func TestWorkspaceMembersAPI_CrossWorkspaceIsolation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-isolation-owner"), "Workspace Owner", "Workspace Isolation Org")
	wsA := seedWorkspaceForAccount(t, app, org, owner, "A", "")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "B", "")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-isolation-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	addRes := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, wsA.ID),
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	listB := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, wsB.ID), nil, ownerTok), app.routes())
	assert.Equal(t, listB.StatusCode, http.StatusOK)
	var payload struct {
		Items []struct {
			AccountID int64 `json:"account_id"`
		} `json:"items"`
	}
	decodeJSONResponse(t, listB.BodyBytes, &payload)
	for _, item := range payload.Items {
		if item.AccountID == member.ID {
			t.Fatal("workspace B should not list workspace A member")
		}
	}

	deleteB := send(t, newAuthRequest(t, http.MethodDelete, workspaceMembersURL(org.Slug, wsB.ID)+"/"+strconv.FormatInt(member.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, deleteB.StatusCode, http.StatusNoContent)
	ids := listWorkspaceIDs(t, app, memberTok, org.Slug)
	if !containsInt64(ids, wsA.ID) || containsInt64(ids, wsB.ID) {
		t.Fatalf("expected only workspace A visibility, got %v", ids)
	}
}

func TestWorkspaceTeamsAPI_AddListRemoveAndDerivedVisibility(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-teams-owner"), "Workspace Owner", "Workspace Teams Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-teams-member"), "Team Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	team, err := app.db.InsertTeam(context.Background(), org.ID, "engineering", "Engineering")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	addRes := send(t, newAuthRequest(t, http.MethodPost, workspaceTeamsURL(org.Slug, ws.ID),
		map[string]any{"team_id": team.ID}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)
	duplicateRes := send(t, newAuthRequest(t, http.MethodPost, workspaceTeamsURL(org.Slug, ws.ID),
		map[string]any{"team_id": team.ID}, ownerTok), app.routes())
	assert.Equal(t, duplicateRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, workspaceTeamsURL(org.Slug, ws.ID)+"?q=engine&sort=slug&order=asc&page=1&page_size=10", nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	var listPayload struct {
		Total int `json:"total"`
		Items []struct {
			TeamID      int64  `json:"team_id"`
			Slug        string `json:"slug"`
			MemberCount int    `json:"member_count"`
		} `json:"items"`
	}
	decodeJSONResponse(t, listRes.BodyBytes, &listPayload)
	assert.Equal(t, listPayload.Total, 1)
	assert.Equal(t, listPayload.Items[0].TeamID, team.ID)
	assert.Equal(t, listPayload.Items[0].Slug, "engineering")
	assert.Equal(t, listPayload.Items[0].MemberCount, 1)

	if !containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("team member should see workspace through workspace team membership")
	}

	deleteRes := send(t, newAuthRequest(t, http.MethodDelete, workspaceTeamsURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(team.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, deleteRes.StatusCode, http.StatusNoContent)
	deleteAgainRes := send(t, newAuthRequest(t, http.MethodDelete, workspaceTeamsURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(team.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, deleteAgainRes.StatusCode, http.StatusNoContent)

	if containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("team member should lose workspace visibility after workspace team removal")
	}
}

func TestWorkspaceTeamsAPI_RejectsCrossOrgTeam(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-teams-cross-owner"), "Workspace Owner", "Workspace Teams Cross Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	_, _, otherOrg := seedOrgOwner(t, app, uniqueEmail(t, "ws-teams-other-owner"), "Other Owner", "Other Workspace Teams Org")
	otherTeam, err := app.db.InsertTeam(context.Background(), otherOrg.ID, "other", "Other Team")
	if err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPost, workspaceTeamsURL(org.Slug, ws.ID),
		map[string]any{"team_id": otherTeam.ID}, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestWorkspaceMembershipAPIPermissions(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-perm-owner"), "Workspace Owner", "Workspace Permissions Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	reader, readerTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-reader"), "Reader")
	writer, writerTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-writer"), "Writer")
	target, _ := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-target"), "Target")
	for _, account := range []int64{reader.ID, writer.ID, target.ID} {
		if err := app.db.AddOrgMember(context.Background(), org.ID, account); err != nil {
			t.Fatal(err)
		}
	}

	readRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", access.PermWsRead, access.PermPolicyRead)
	modifyRoleID := createRoleForTest(t, app, org.ID, &ws.ID, "workspace", access.PermWsRead, access.PermPolicyModify)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), readRoleID, "account", reader.ID, "workspace", 0).StatusCode, http.StatusNoContent)
	assert.Equal(t, grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(ws.ID, 10), modifyRoleID, "account", writer.ID, "workspace", 0).StatusCode, http.StatusNoContent)

	readList := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, ws.ID), nil, readerTok), app.routes())
	assert.Equal(t, readList.StatusCode, http.StatusOK)
	readAdd := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID), map[string]any{"account_id": target.ID}, readerTok), app.routes())
	assert.Equal(t, readAdd.StatusCode, http.StatusForbidden)

	writeList := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, ws.ID), nil, writerTok), app.routes())
	assert.Equal(t, writeList.StatusCode, http.StatusForbidden)
	writeAdd := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID), map[string]any{"account_id": target.ID}, writerTok), app.routes())
	assert.Equal(t, writeAdd.StatusCode, http.StatusNoContent)
	writeDelete := send(t, newAuthRequest(t, http.MethodDelete, workspaceMembersURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(target.ID, 10), nil, writerTok), app.routes())
	assert.Equal(t, writeDelete.StatusCode, http.StatusNoContent)
}

func TestWorkspaceMembersDefaultPolicyCanBeRevoked(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-revoke-owner"), "Workspace Owner", "Workspace Revoke Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary", "")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-revoke-member"), "Workspace Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	bindingID := findWorkspacePolicyBindingID(t, app, ownerTok, org.Slug, ws.ID, access.SubjectTypeWorkspaceMembers, access.BuiltinWorkspaceMemberRole)
	revokeRes := send(t, newAuthRequest(t, http.MethodDelete, workspacePoliciesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(bindingID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, revokeRes.StatusCode, http.StatusNoContent)

	addRes := send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, ws.ID),
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	if containsInt64(listWorkspaceIDs(t, app, memberTok, org.Slug), ws.ID) {
		t.Fatal("workspace membership should not grant visibility after default workspace_members policy is revoked")
	}

	listRes := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, ws.ID), nil, ownerTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
}

func TestWorkspaceMembersSubjectCanOnlyTargetCurrentWorkspace(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-subject-owner"), "Workspace Owner", "Workspace Subject Org")
	wsA := seedWorkspaceForAccount(t, app, org, owner, "A", "")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "B", "")
	roleID := createRoleForTest(t, app, org.ID, &wsA.ID, "workspace", access.PermWsRead)

	okRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsA.ID, "workspace", 0)
	assert.Equal(t, okRes.StatusCode, http.StatusNoContent)

	crossRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsB.ID, "workspace", 0)
	assert.Equal(t, crossRes.StatusCode, http.StatusNotFound)

	orgPolicyRes := grantOrgPolicyRole(t, app, ownerTok, org.Slug, roleID, access.SubjectTypeWorkspaceMembers, wsA.ID)
	assert.Equal(t, orgPolicyRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, orgPolicyRes, "subject_type")
}

func TestWorkspaceMembersSubjectCanBindEnvironmentOnlyInsideCurrentWorkspace(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-env-subject-owner"), "Workspace Owner", "Workspace Env Subject Org")
	wsA := seedWorkspaceForAccount(t, app, org, owner, "A", "")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "B", "")
	envA := seedEnvironment(t, app, wsA.ID, org.ID, "Env A")
	envB := seedEnvironment(t, app, wsB.ID, org.ID, "Env B")
	roleID := createRoleForTest(t, app, org.ID, nil, "environment", access.PermEnvRead)

	okRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsA.ID, "environment", envA.ID)
	assert.Equal(t, okRes.StatusCode, http.StatusNoContent)

	crossRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsA.ID, "environment", envB.ID)
	assert.Equal(t, crossRes.StatusCode, http.StatusNotFound)

	wrongSubjectRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsB.ID, "environment", envA.ID)
	assert.Equal(t, wrongSubjectRes.StatusCode, http.StatusNotFound)
}

func TestWorkspaceMembersSubjectCanBindConnectionOnlyInsideCurrentWorkspace(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-conn-subject-owner"), "Workspace Owner", "Workspace Conn Subject Org")
	wsA := seedWorkspaceForAccount(t, app, org, owner, "A", "")
	wsB := seedWorkspaceForAccount(t, app, org, owner, "B", "")
	connA, err := app.db.InsertConnection(context.Background(), wsA.ID, nil, "Conn A", "postgres", "enc-a", "open")
	if err != nil {
		t.Fatal(err)
	}
	connB, err := app.db.InsertConnection(context.Background(), wsB.ID, nil, "Conn B", "postgres", "enc-b", "open")
	if err != nil {
		t.Fatal(err)
	}
	roleID := createRoleForTest(t, app, org.ID, nil, "connection", access.PermConnRead)

	okRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsA.ID, "connection", connA.ID)
	assert.Equal(t, okRes.StatusCode, http.StatusNoContent)

	crossRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsA.ID, "connection", connB.ID)
	assert.Equal(t, crossRes.StatusCode, http.StatusNotFound)

	wrongSubjectRes := grantWorkspacePolicyRole(t, app, ownerTok, org.Slug, strconv.FormatInt(wsA.ID, 10), roleID, access.SubjectTypeWorkspaceMembers, wsB.ID, "connection", connA.ID)
	assert.Equal(t, wrongSubjectRes.StatusCode, http.StatusNotFound)
}

func TestRemovingOrgMemberClearsWorkspaceAndTeamDerivedMembership(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	owner, ownerTok, org := seedOrgOwner(t, app, uniqueEmail(t, "ws-members-org-remove-owner"), "Workspace Owner", "Workspace Org Remove Org")
	wsDirect := seedWorkspaceForAccount(t, app, org, owner, "Direct", "")
	wsTeam := seedWorkspaceForAccount(t, app, org, owner, "Team", "")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "ws-members-org-remove-member"), "Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	team, err := app.db.InsertTeam(context.Background(), org.ID, "cleanup", "Cleanup")
	if err != nil {
		t.Fatal(err)
	}
	if err = app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, send(t, newAuthRequest(t, http.MethodPost, workspaceMembersURL(org.Slug, wsDirect.ID), map[string]any{"account_id": member.ID}, ownerTok), app.routes()).StatusCode, http.StatusNoContent)
	assert.Equal(t, send(t, newAuthRequest(t, http.MethodPost, workspaceTeamsURL(org.Slug, wsTeam.ID), map[string]any{"team_id": team.ID}, ownerTok), app.routes()).StatusCode, http.StatusNoContent)
	ids := listWorkspaceIDs(t, app, memberTok, org.Slug)
	if !containsInt64(ids, wsDirect.ID) || !containsInt64(ids, wsTeam.ID) {
		t.Fatalf("expected member to see both workspaces before org removal, got %v", ids)
	}

	removeOrgRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/members/"+strconv.FormatInt(member.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, removeOrgRes.StatusCode, http.StatusNoContent)

	directList := send(t, newAuthRequest(t, http.MethodGet, workspaceMembersURL(org.Slug, wsDirect.ID), nil, ownerTok), app.routes())
	assert.Equal(t, directList.StatusCode, http.StatusOK)
	var directPayload struct {
		Items []struct {
			AccountID int64 `json:"account_id"`
		} `json:"items"`
	}
	decodeJSONResponse(t, directList.BodyBytes, &directPayload)
	for _, item := range directPayload.Items {
		if item.AccountID == member.ID {
			t.Fatal("removed org member should be removed from direct workspace memberships")
		}
	}

	teamMembers := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/teams/cleanup/members", nil, ownerTok), app.routes())
	assert.Equal(t, teamMembers.StatusCode, http.StatusOK)
	var teamPayload struct {
		Items []struct {
			AccountID int64 `json:"account_id"`
		} `json:"items"`
	}
	decodeJSONResponse(t, teamMembers.BodyBytes, &teamPayload)
	for _, item := range teamPayload.Items {
		if item.AccountID == member.ID {
			t.Fatal("removed org member should be removed from org team memberships")
		}
	}
}
