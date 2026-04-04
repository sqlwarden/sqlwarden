package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

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

	var members []map[string]any
	err := json.Unmarshal(res.BodyBytes, &members)
	if err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}
	if len(members) < 1 {
		t.Fatal("expected at least 1 member (the owner)")
	}
}

func TestListOrgMembers_SupportsSearchAndSort(t *testing.T) {
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

	res := send(t, newOrgRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members?q=ali&sort=name&order=asc", ownerTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var members []map[string]any
	decodeJSONResponse(t, res.BodyBytes, &members)
	assert.Equal(t, len(members), 1)
	assert.Equal(t, members[0]["name"], "Alice Analyst")
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
		"role":  "member",
	})
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Non-existent email returns 404.
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "nobody@example.com",
		"role":  "member",
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
		"role":  "member",
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Promote member to admin.
	req := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+memberID, map[string]any{
		"role": "admin",
	})
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Demoting last owner should fail with 422.
	req2 := newTestRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+ownerID, map[string]any{
		"role": "member",
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
		map[string]any{"role": "admin"}, ownerTok), app.routes())
	assert.Equal(t, promoteRes.StatusCode, http.StatusNoContent)

	promoteAgainRes := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug+"/members/"+memberID,
		map[string]any{"role": "owner"}, ownerTok), app.routes())
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
		"role":  "member",
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
		"role":  "member",
	})
	addReq.Header.Set("Authorization", "Bearer "+ownerTok)
	addRes := send(t, addReq, app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	// Member cannot list members (requires members:read permission).
	req := newTestRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/members", nil)
	req.Header.Set("Authorization", "Bearer "+memberTok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)

	// Member cannot add members (requires members:write permission).
	req2 := newTestRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/members", map[string]any{
		"email": "another@example.com",
		"role":  "member",
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

func TestUpdateOrganization_IsExplicitlyUnsupported(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-update-owner"), "Org Owner", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+slug,
		map[string]any{"name": "Renamed Org"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusMethodNotAllowed)
}

func TestDeleteOrganization_IsExplicitlyUnsupported(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	_, tok, slug := registerAndLogin(t, app, uniqueEmail(t, "org-delete-owner"), "Org Owner", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug, nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusMethodNotAllowed)
}
