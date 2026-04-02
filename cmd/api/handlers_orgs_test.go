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

// registerAndLogin registers a user, logs in, creates an org, and returns the account ID, access token, and org slug.
// For the very first account on a fresh instance it uses POST /api/setup (which also makes the account an instance
// admin). For subsequent accounts it uses POST /api/v1/auth/register + login, which is allowed once setup is done.
func registerAndLogin(t *testing.T, app *application, email, name, password string) (accountID, accessToken, orgSlug string) {
	t.Helper()

	// Try setup first; if the instance is already configured (409) fall back to regular register + login.
	setupRes := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email":    email,
		"name":     name,
		"password": password,
	}), app.routes())

	if setupRes.StatusCode == http.StatusCreated {
		accountID = fmt.Sprintf("%v", setupRes.BodyFields["account"].(map[string]any)["id"])
		accessToken = setupRes.BodyFields["access_token"].(string)
	} else if setupRes.StatusCode == http.StatusConflict {
		regRes := registerTestUser(t, app, email, name, password)
		if regRes.StatusCode != http.StatusCreated {
			t.Fatalf("registerAndLogin: register failed (%d): %s", regRes.StatusCode, regRes.BodyBytes)
		}
		accountID = fmt.Sprintf("%v", regRes.BodyFields["id"])

		// Grant instance admin so this test user can create orgs (test infrastructure only).
		idNum, _ := strconv.ParseInt(accountID, 10, 64)
		if err := app.db.InsertInstanceAdmin(context.Background(), idNum); err != nil {
			t.Fatalf("registerAndLogin: InsertInstanceAdmin: %v", err)
		}

		loginRes := loginTestUser(t, app, email, password)
		if loginRes.StatusCode != http.StatusOK {
			t.Fatalf("registerAndLogin: login failed (%d): %s", loginRes.StatusCode, loginRes.BodyBytes)
		}
		accessToken = extractAccessToken(t, loginRes)
	} else {
		t.Fatalf("registerAndLogin: setup returned unexpected status %d: %s", setupRes.StatusCode, setupRes.BodyBytes)
	}

	// Create a personal org for the user.
	orgName := name + "'s Org"
	orgReq := newTestRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": orgName})
	orgReq.Header.Set("Authorization", "Bearer "+accessToken)
	orgRes := send(t, orgReq, app.routes())
	if orgRes.StatusCode != http.StatusCreated {
		t.Fatalf("registerAndLogin: failed to create org, got %d: %s", orgRes.StatusCode, orgRes.BodyBytes)
	}
	orgSlug = orgRes.BodyFields["slug"].(string)

	return accountID, accessToken, orgSlug
}

func TestGetOrg(t *testing.T) {
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

func TestAddOrgMember(t *testing.T) {
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

func TestRemoveOrgMember(t *testing.T) {
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
	app := newTestApp(t)
	_, tok, _ := registerAndLogin(t, app, "dup-org@example.com", "User", "securepass99")

	body := map[string]any{"name": "Duplicate Org"}
	res1 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", body, tok), app.routes())
	assert.Equal(t, res1.StatusCode, http.StatusCreated)

	res2 := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", body, tok), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}
