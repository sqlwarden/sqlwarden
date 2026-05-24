package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
)

// registerAndLogin seeds an instance-admin account, creates an org, and returns the account ID, access token, and org slug.
func registerAndLogin(t *testing.T, app *application, email, name, password string) (accountID, accessToken, orgSlug string) {
	t.Helper()
	account, tok, org := seedOrgOwner(t, app, email, name, name+"'s Org")
	accountID = strconv.FormatInt(account.ID, 10)
	accessToken = tok
	orgSlug = org.Slug
	return accountID, accessToken, orgSlug
}

func TestGetOrg(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "getorg@example.com", "GetOrg User", "securepass99")

	// Valid request.
	req := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["slug"].(string), slug)

	// Unknown slug returns 404.
	req2 := newTestRequest(t, http.MethodGet, "/api/v1/orgs/nonexistent-slug", nil)
	req2.Header.Set("Authorization", "Bearer "+tok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusNotFound)
}

func TestListOrgMembers(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, tok, slug := registerAndLogin(t, app, "listmem@example.com", "ListMem User", "securepass99")

	req := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	err := json.Unmarshal(res.BodyBytes, &payload)
	if err != nil {
		t.Fatalf("expected paginated JSON object: %v", err)
	}
	if payload.Page != 1 || payload.PageSize != 25 {
		t.Fatalf("unexpected defaults: page=%d page_size=%d", payload.Page, payload.PageSize)
	}
	if len(payload.Items) < 1 {
		t.Fatal("expected at least 1 member (the owner)")
	}
}

func TestListOrgMembers_SupportsPaginationSearchFilterAndSort(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, ownerTok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-members-owner"), "Owner User", "securepass99")

	alice, _ := seedAccountWithToken(t, app, uniqueEmail(t, "alice-member"), "Alice Analyst")
	bob, _ := seedAccountWithToken(t, app, uniqueEmail(t, "bob-member"), "Bob Builder")

	for _, email := range []string{alice.Email, bob.Email} {
		res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
			map[string]any{"email": email}, ownerTok), app.routes())
		assert.Equal(t, res.StatusCode, http.StatusNoContent)
	}

	addRes := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+strconv.FormatInt(alice.ID, 10),
		map[string]any{"role": access.BuiltinOrgAdminRole}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	res := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members?q=ali&role=Organization%20Admin&sort=name&order=asc&page=1&page_size=1", ownerTok), app.routes())
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
	assert.Equal(t, payload.Items[0]["name"], "Alice Analyst")
	assert.Equal(t, payload.Items[0]["role"], access.BuiltinOrgAdminRole)
}

func TestListOrgMemberCandidates(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, ownerTok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-candidates-owner"), "Owner User", "securepass99")
	candidate, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-candidates-alice"), "Alice Candidate")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-candidates-bob"), "Bob Member")

	addMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addMemberRes.StatusCode, http.StatusNoContent)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members/candidates?q="+url.QueryEscape(candidate.Email)+"&sort=name&order=asc&page=1&page_size=10", nil, ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, res.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["email"].(string), candidate.Email)
}

func TestAddOrgMemberByAccountID(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, ownerTok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-add-id-owner"), "Owner User", "securepass99")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-add-id-member"), "Member User")

	addMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addMemberRes.StatusCode, http.StatusNoContent)

	getMemberRes := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members/"+strconv.FormatInt(member.ID, 10), ownerTok), app.routes())
	assert.Equal(t, getMemberRes.StatusCode, http.StatusOK)
	assert.Equal(t, getMemberRes.BodyFields["email"].(string), member.Email)
}

func TestGetOrgMemberAndTeams(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, ownerTok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-member-context-owner"), "Owner User", "securepass99")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "org-member-context-member"), "Member User")

	addMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
		map[string]any{"email": member.Email}, ownerTok), app.routes())
	assert.Equal(t, addMemberRes.StatusCode, http.StatusNoContent)

	createTeamRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams",
		map[string]any{"slug": "engineering", "name": "Engineering"}, ownerTok), app.routes())
	assert.Equal(t, createTeamRes.StatusCode, http.StatusCreated)

	addTeamMemberRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/teams/engineering/members",
		map[string]any{"account_id": member.ID}, ownerTok), app.routes())
	assert.Equal(t, addTeamMemberRes.StatusCode, http.StatusNoContent)

	getMemberRes := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members/"+strconv.FormatInt(member.ID, 10), ownerTok), app.routes())
	assert.Equal(t, getMemberRes.StatusCode, http.StatusOK)
	assert.Equal(t, getMemberRes.BodyFields["email"].(string), member.Email)
	assert.Equal(t, getMemberRes.BodyFields["role"].(string), access.BuiltinOrgMemberRole)

	teamsRes := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members/"+strconv.FormatInt(member.ID, 10)+"/teams", ownerTok), app.routes())
	assert.Equal(t, teamsRes.StatusCode, http.StatusOK)
	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	decodeJSONResponse(t, teamsRes.BodyBytes, &payload)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, payload.Items[0]["slug"], "engineering")
}

func TestAddOrgMember(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// Owner user.
	_, ownerTok, slug := registerAndLogin(t, app, "owner-add@example.com", "Owner", "securepass99")

	// Second user to be added.
	registerAndLogin(t, app, "member-add@example.com", "Member", "securepass99")

	// Add second user as member.
	req := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "member-add@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Non-existent email returns 404.
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "nobody@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	req2.Header.Set("Authorization", "Bearer "+ownerTok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusNotFound)
}

func TestUpdateOrgMemberRole(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	ownerID, ownerTok, slug := registerAndLogin(t, app, "owner-upd@example.com", "Owner", "securepass99")

	// Add a second user.
	memberID, _, _ := registerAndLogin(t, app, "member-upd@example.com", "Member", "securepass99")

	addReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "member-upd@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Promote member to admin.
	req := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+memberID, map[string]any{
		"role": access.BuiltinOrgAdminRole,
	})
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Demoting last owner should fail with 422.
	req2 := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+ownerID, map[string]any{
		"role": access.BuiltinOrgMemberRole,
	})
	req2.Header.Set("Authorization", "Bearer "+ownerTok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestUpdateOrgMemberRoleReplacesPreviousBuiltinBinding(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	_, ownerTok, slug := registerAndLogin(t, app, "owner-switch@example.com", "Owner", "securepass99")
	memberID, _, _ := registerAndLogin(t, app, "member-switch@example.com", "Member", "securepass99")

	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members",
		map[string]any{"email": "member-switch@example.com"}, ownerTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	promoteRes := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+memberID,
		map[string]any{"role": access.BuiltinOrgAdminRole}, ownerTok), app.routes())
	assert.Equal(t, promoteRes.StatusCode, http.StatusNoContent)

	promoteAgainRes := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+memberID,
		map[string]any{"role": access.BuiltinOrgOwnerRole}, ownerTok), app.routes())
	assert.Equal(t, promoteAgainRes.StatusCode, http.StatusNoContent)

	org, found, err := app.db.GetOrgBySlug(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected org to exist")
	}

	count, err := app.db.NewSelect().
		TableExpr("role_bindings").
		Where("org_id = ? AND subject_type = 'account' AND subject_id = ? AND resource_type = 'org' AND resource_id = ?",
			org.ID, mustParseInt64(t, memberID), org.ID).
		Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 1)
}

func mustParseInt64(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRemoveOrgMember(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	ownerID, ownerTok, slug := registerAndLogin(t, app, "owner-rem@example.com", "Owner", "securepass99")

	// Add a second user.
	memberID, _, _ := registerAndLogin(t, app, "member-rem@example.com", "Member", "securepass99")

	addReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "member-rem@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Remove member — should succeed.
	req := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/members/"+memberID, nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Removing last owner should fail with 422.
	req2 := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/members/"+ownerID, nil)
	req2.Header.Set("Authorization", "Bearer "+ownerTok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestOrgPermissionEnforcement(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// Owner registers and gets an org.
	_, ownerTok, slug := registerAndLogin(t, app, "owner-perm@example.com", "Owner", "securepass99")

	// Register a second user and add as member.
	_, memberTok, _ := registerAndLogin(t, app, "member-perm@example.com", "Member", "securepass99")

	addReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "member-perm@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Member can list members through the default org_members -> member -> org:read policy.
	req := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+memberTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	// Member cannot add members (requires members:write permission).
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "another@example.com",
		"role":  access.BuiltinOrgMemberRole,
	})
	req2.Header.Set("Authorization", "Bearer "+memberTok)
	res2 := send(t, req2, app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusForbidden)
}

func TestCreateOrgDuplicateName(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := registerAndLogin(t, app, "dup-org@example.com", "User", "securepass99")

	body := map[string]any{"name": "Duplicate Org"}
	res1 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", body, tok), app.routes())
	assert.Equal(t, res1.StatusCode, http.StatusCreated)

	res2 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", body, tok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestCreateOrgWithExplicitSlug(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := registerAndLogin(t, app, "custom-org@example.com", "User", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{
		"name": "Custom Org",
		"slug": "custom-org",
	}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	assert.Equal(t, res.BodyFields["slug"], "custom-org")
}

func TestCreateOrgDuplicateSlug(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, tok, _ := registerAndLogin(t, app, "dup-slug@example.com", "User", "securepass99")

	res1 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{
		"name": "Alpha Org",
		"slug": "shared-slug",
	}, tok), app.routes())
	assert.Equal(t, res1.StatusCode, http.StatusCreated)

	res2 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{
		"name": "Beta Org",
		"slug": "shared-slug",
	}, tok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestUpdateOrganization(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-update-owner"), "Org Owner", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug,
		map[string]any{"name": "Renamed Org"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["name"], "Renamed Org")
	assert.Equal(t, res.BodyFields["slug"], any(slug))

	getRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["name"], "Renamed Org")
}

func TestUpdateOrganizationRequiresOrgWrite(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, _, org := seedOrgOwner(t, app, uniqueEmail(t, "org-update-forbidden-owner"), "Org Owner", "Org Update Forbidden")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "org-update-forbidden-member"), "Org Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug,
		map[string]any{"name": "Renamed Org"}, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestDeleteOrganization(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-delete-owner"), "Org Owner", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	getRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug, nil, tok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusNotFound)
}

func TestDeleteOrganizationRequiresOrgDelete(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, _, org := seedOrgOwner(t, app, uniqueEmail(t, "org-delete-forbidden-owner"), "Org Owner", "Org Delete Forbidden")
	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "org-delete-forbidden-member"), "Org Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+org.Slug, nil, memberTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestDeleteOrganizationIgnoresCrossOrgDeletePermission(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	account, accountTok := seedAccountWithToken(t, app, uniqueEmail(t, "org-delete-cross-account"), "Cross Delete")
	_, ownerATok, orgA := seedOrgOwner(t, app, uniqueEmail(t, "org-delete-cross-owner-a"), "Org A Owner", "Delete Cross A")
	_, ownerBTok, orgB := seedOrgOwner(t, app, uniqueEmail(t, "org-delete-cross-owner-b"), "Org B Owner", "Delete Cross B")

	if err := app.db.AddOrgMember(context.Background(), orgA.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddOrgMember(context.Background(), orgB.ID, account.ID); err != nil {
		t.Fatal(err)
	}

	deleteRoleB := createRoleForTest(t, app, orgB.ID, nil, "org", "org:delete")
	res := grantOrgPolicyRole(t, app, ownerBTok, orgB.Slug, deleteRoleB, "account", account.ID)
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	deleteARes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+orgA.Slug, nil, accountTok), app.routes())
	assert.Equal(t, deleteARes.StatusCode, http.StatusForbidden)

	getARes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+orgA.Slug, nil, ownerATok), app.routes())
	assert.Equal(t, getARes.StatusCode, http.StatusOK)
	assert.Equal(t, getARes.BodyFields["slug"], any(orgA.Slug))
}
