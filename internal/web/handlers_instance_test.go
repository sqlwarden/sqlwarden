package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

// setupInstance calls POST /api/setup and returns the access token.
func setupInstance(t *testing.T, app *application, email, name, password string) string {
	t.Helper()
	res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email":    email,
		"name":     name,
		"password": password,
	}), app.routes())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("setupInstance: got %d: %s", res.StatusCode, res.BodyBytes)
	}
	tok, ok := res.BodyFields["access_token"].(string)
	if !ok {
		t.Fatal("setupInstance: access_token missing from response")
	}
	return tok
}

func TestSetupCreatesFirstInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email":    "admin@example.com",
		"name":     "Admin",
		"password": "securepass99",
	}), app.routes())

	assert.Equal(t, res.StatusCode, http.StatusCreated)
	_, hasToken := res.BodyFields["access_token"]
	assert.Equal(t, hasToken, true)
	_, hasAccount := res.BodyFields["account"]
	assert.Equal(t, hasAccount, true)
}

func TestSetupStatus(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	before := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
	assert.Equal(t, before.StatusCode, http.StatusOK)
	assert.Equal(t, before.BodyFields["configured"], false)

	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	after := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
	assert.Equal(t, after.StatusCode, http.StatusOK)
	assert.Equal(t, after.BodyFields["configured"], true)
}

func TestSetupStatusUnaffectedByRegularAccounts(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	account := seedAccount(t, app, "nonadmin@example.com", "Non Admin")
	if account.ID == 0 {
		t.Fatal("expected seeded account")
	}

	res := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["configured"], false)
}

func TestSetupStatus_ReturnsStableShape(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	res := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assertBodyContainsJSONKeys(t, res.BodyBytes, "configured")
}

func TestSetupBlockedAfterFirstAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// First setup succeeds.
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// Second setup is rejected.
	res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email":    "admin2@example.com",
		"name":     "Admin2",
		"password": "securepass99",
	}), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusConflict)
}

func TestSetupValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// Missing fields.
	res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email": "admin@example.com",
		// missing name and password
	}), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)

	// Short password.
	res2 := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
		"email":    "admin@example.com",
		"name":     "Admin",
		"password": "short",
	}), app.routes())
	assert.Equal(t, res2.StatusCode, http.StatusUnprocessableEntity)
}

func TestCreateOrgRequiresInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	// Setup an instance admin.
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// A regular (non-admin) user cannot create orgs.
	regRes := registerTestUser(t, app, "regular@example.com", "Regular", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	assert.Equal(t, loginRes.StatusCode, http.StatusOK)
	tok := extractAccessToken(t, loginRes)

	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "MyOrg"}, tok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusForbidden)
}

func TestCreateOrgAllowedForInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "MyOrg"}, adminTok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusCreated)
	assert.Equal(t, orgRes.BodyFields["name"].(string), "MyOrg")
}

func TestCreateOrgAllowedForInstanceAdminWithCustomSlug(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{
		"name": "My Org",
		"slug": "my-org",
	}, adminTok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusCreated)
	assert.Equal(t, orgRes.BodyFields["slug"].(string), "my-org")
}

func TestListInstanceAdmins(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var admins struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(res.BodyBytes, &admins); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, admins.Total, 1)
	assert.Equal(t, len(admins.Items), 1)
}

func TestListInstanceAdminsAfterPromotion(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	regRes := registerTestUser(t, app, "list-second@example.com", "List Second", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)

	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "list-second@example.com"}, adminTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, adminTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)

	var admins struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &admins); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, admins.Total, 2)
	assert.Equal(t, len(admins.Items), 2)
}

func TestListInstanceAdmins_SupportsPaginationAndSort(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	regRes := registerTestUser(t, app, "third@example.com", "Third", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)

	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "third@example.com"}, adminTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins?sort=account_id&order=desc&page=1&page_size=1", nil, adminTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 2)

	items := listRes.BodyFields["items"].([]any)
	assert.Equal(t, len(items), 1)
}

func TestListInstanceAdmins_SupportsSearch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	regRes := registerTestUser(t, app, "searchable@example.com", "Searchable User", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)

	addRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "searchable@example.com"}, adminTok), app.routes())
	assert.Equal(t, addRes.StatusCode, http.StatusNoContent)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins?q=searchable", nil, adminTok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusOK)
	assert.Equal(t, int(listRes.BodyFields["total"].(float64)), 1)
}

func TestListInstanceAdminsRequiresInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	regRes := registerTestUser(t, app, "regular@example.com", "Regular", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestListInstanceAdminsRequiresAuth(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	res := send(t, newTestRequest(t, http.MethodGet, "/api/v1/instance/admins", nil), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestGetInstanceSettings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/settings", nil, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["personal_spaces_enabled"], true)
}

func TestUpdateInstanceSettings(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings",
		map[string]any{"personal_spaces_enabled": false}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["personal_spaces_enabled"], false)

	getRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/settings", nil, adminTok), app.routes())
	assert.Equal(t, getRes.StatusCode, http.StatusOK)
	assert.Equal(t, getRes.BodyFields["personal_spaces_enabled"], false)
}

func TestUpdateInstanceSettingsRequiresInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	regRes := registerTestUser(t, app, "settings-user@example.com", "Settings User", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "settings-user@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings",
		map[string]any{"personal_spaces_enabled": false}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestListOrganizations(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "Alpha Team"}, adminTok), app.routes())
	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "Zeta Labs"}, adminTok), app.routes())

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/orgs?q=zeta&slug=zeta-labs&sort=name&order=desc&page=1&page_size=1", nil, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(res.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 1)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["name"], "Zeta Labs")
	if payload.Items[0]["member_count"] != float64(1) {
		t.Fatalf("expected member_count=1, got %v", payload.Items[0]["member_count"])
	}
	if payload.Items[0]["team_count"] != float64(0) {
		t.Fatalf("expected team_count=0, got %v", payload.Items[0]["team_count"])
	}
}

func TestListOrganizationsRequiresInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	registerTestUser(t, app, "regular@example.com", "Regular", "securepass99")
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/orgs", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestListInstanceAccounts(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	assert.Equal(t, registerTestUser(t, app, "zeta-account@example.com", "Zeta Account", "securepass99").StatusCode, http.StatusCreated)
	assert.Equal(t, registerTestUser(t, app, "alpha-account@example.com", "Alpha Account", "securepass99").StatusCode, http.StatusCreated)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/accounts?q=account&sort=email&order=asc&page=1&page_size=1", nil, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	var payload struct {
		Items    []map[string]any `json:"items"`
		Page     int              `json:"page"`
		PageSize int              `json:"page_size"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal(res.BodyBytes, &payload); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, payload.Page, 1)
	assert.Equal(t, payload.PageSize, 1)
	assert.Equal(t, payload.Total, 2)
	assert.Equal(t, len(payload.Items), 1)
	assert.Equal(t, payload.Items[0]["email"], "alpha-account@example.com")
}

func TestCreateInstanceAccount(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/accounts",
		map[string]any{"email": "created@example.com", "name": "Created User", "password": "securepass99"}, adminTok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusCreated)
	assert.Equal(t, createRes.BodyFields["email"], "created@example.com")
	assert.Equal(t, createRes.BodyFields["name"], "Created User")

	loginRes := loginTestUser(t, app, "created@example.com", "securepass99")
	assert.Equal(t, loginRes.StatusCode, http.StatusOK)

	account, found, err := app.db.GetAccountByEmail(t.Context(), "created@example.com")
	assert.Nil(t, err)
	assert.True(t, found)
	orgs, err := app.db.ListAccountOrgsPage(t.Context(), database.ListAccountOrgsParams{AccountID: account.ID, Page: 1, PageSize: 10})
	assert.Nil(t, err)
	assert.Equal(t, orgs.Total, 0)
}

func TestCreateInstanceAccountDuplicateAndValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	duplicateRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/accounts",
		map[string]any{"email": "admin@example.com", "name": "Admin Again", "password": "securepass99"}, adminTok), app.routes())
	assert.Equal(t, duplicateRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, duplicateRes, "email")

	invalidRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/accounts",
		map[string]any{"email": "not-an-email", "name": "", "password": "short"}, adminTok), app.routes())
	assert.Equal(t, invalidRes.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, invalidRes, "email")
	assertValidationField(t, invalidRes, "name")
	assertValidationField(t, invalidRes, "password")
}

func TestInstanceAccountsRequireInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")
	assert.Equal(t, registerTestUser(t, app, "regular@example.com", "Regular", "securepass99").StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/accounts", nil, tok), app.routes())
	assert.Equal(t, listRes.StatusCode, http.StatusForbidden)
	createRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/accounts",
		map[string]any{"email": "created@example.com", "name": "Created User", "password": "securepass99"}, tok), app.routes())
	assert.Equal(t, createRes.StatusCode, http.StatusForbidden)
}

func TestAddInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// Register a second user.
	regRes := registerTestUser(t, app, "second@example.com", "Second", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)

	// Add them as instance admin.
	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "second@example.com"}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	// Now they show up in the list.
	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, adminTok), app.routes())
	var admins struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &admins); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, admins.Total, 2)
	assert.Equal(t, len(admins.Items), 2)
}

func TestAddInstanceAdminDuplicateIsIdempotent(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "admin@example.com"}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)
}

func TestAddInstanceAdminUnknownEmail(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "nobody@example.com"}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func TestAddInstanceAdminValidation(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{}, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
}

func TestAddInstanceAdminRequiresInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	regRes := registerTestUser(t, app, "regular@example.com", "Regular", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "admin@example.com"}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestRemoveInstanceAdmin(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// Add a second admin.
	regRes := registerTestUser(t, app, "second@example.com", "Second", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	secondID := fmt.Sprintf("%v", regRes.BodyFields["id"])

	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "second@example.com"}, adminTok), app.routes())

	// Remove the second admin.
	delRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/instance/admins/"+secondID, nil, adminTok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNoContent)

	// Back to one admin.
	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, adminTok), app.routes())
	var admins struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &admins); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, admins.Total, 1)
	assert.Equal(t, len(admins.Items), 1)
}

func TestRemoveLastInstanceAdminBlocked(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// Get the admin account ID from the list.
	listRes := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/admins", nil, adminTok), app.routes())
	var admins struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(listRes.BodyBytes, &admins); err != nil {
		t.Fatal(err)
	}
	adminID := fmt.Sprintf("%v", admins.Items[0]["account_id"])

	delRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/instance/admins/"+adminID, nil, adminTok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusUnprocessableEntity)
}

func TestRemoveInstanceAdminInvalidID(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	delRes := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/instance/admins/not-a-number", nil, adminTok), app.routes())
	assert.Equal(t, delRes.StatusCode, http.StatusNotFound)
}

func TestAddedInstanceAdminCanCreateOrg(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)

	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	// Register a second user and promote them.
	regRes := registerTestUser(t, app, "promoted@example.com", "Promoted", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)

	send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/admins",
		map[string]any{"email": "promoted@example.com"}, adminTok), app.routes())

	// Second user logs in and creates an org.
	loginRes := loginTestUser(t, app, "promoted@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	orgRes := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs", map[string]any{"name": "PromotedOrg"}, tok), app.routes())
	assert.Equal(t, orgRes.StatusCode, http.StatusCreated)
}
