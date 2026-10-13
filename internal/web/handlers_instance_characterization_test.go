package web

import (
	"net/http"
	"strconv"
	"testing"
)

func instanceRouteCases() []accessCase {
	return []accessCase{
		{name: "empty bearer token", who: "", want: expectInvalidToken},
		{name: "no authorization header", who: "anonymous", want: accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}},
		{name: "org member who is not an instance admin", who: "member", want: expectNotPermitted},
		{name: "account outside every org", who: "nonMember", want: expectNotPermitted},
	}
}

func instancePath(suffix string) func(*accessCharacterizationFixture) string {
	return func(*accessCharacterizationFixture) string { return "/api/v1/instance" + suffix }
}

func withInstanceCases(extra ...accessCase) []accessCase {
	return append(instanceRouteCases(), extra...)
}

func TestInstanceRoutesCharacterization(t *testing.T) {
	accountSessions := func(f *accessCharacterizationFixture) string {
		return "/api/v1/instance/accounts/" + strconv.FormatInt(f.target.ID, 10) + "/sessions"
	}
	badAccount := func(suffix string) accessCase {
		return accessCase{name: "non-numeric account id", who: "owner", path: instancePath("/accounts/abc" + suffix), want: expectNotFound}
	}
	unknownAccount := func(suffix string) accessCase {
		return accessCase{name: "unknown account", who: "owner", path: instancePath("/accounts/999999" + suffix), want: expectNotFound}
	}

	routes := []accessRoute{
		{name: "list instance admins", method: http.MethodGet, path: instancePath("/admins"), skipStandard: true, extra: instanceRouteCases(), allow: expectStatus(http.StatusOK)},
		{name: "list instance accounts", method: http.MethodGet, path: instancePath("/accounts"), skipStandard: true, extra: instanceRouteCases(), allow: expectStatus(http.StatusOK)},
		{name: "list instance organizations", method: http.MethodGet, path: instancePath("/orgs"), skipStandard: true, extra: instanceRouteCases(), allow: expectStatus(http.StatusOK)},
		{name: "get instance settings", method: http.MethodGet, path: instancePath("/settings"), skipStandard: true, extra: instanceRouteCases(), allow: expectStatus(http.StatusOK)},
		{name: "get instance configuration", method: http.MethodGet, path: instancePath("/configuration"), skipStandard: true, extra: instanceRouteCases(), allow: expectStatus(http.StatusOK)},
		{
			name: "get instance capabilities", method: http.MethodGet, path: instancePath("/capabilities"), skipStandard: true,
			extra: []accessCase{
				{name: "empty bearer token", who: "", want: expectInvalidToken},
				{name: "no authorization header", who: "anonymous", want: accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}},
				{name: "org member who is not an instance admin", who: "member", want: expectStatus(http.StatusOK)},
				{name: "account outside every org", who: "nonMember", want: expectStatus(http.StatusOK)},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "update instance settings", method: http.MethodPatch, path: instancePath("/settings"), skipStandard: true,
			body: staticBody(map[string]any{"instance_name": "Characterized Instance"}),
			extra: withInstanceCases(
				accessCase{name: "no fields", who: "owner", body: staticBody(map[string]any{}), want: expectValidation},
				accessCase{name: "invalid log level", who: "owner", body: staticBody(map[string]any{"log_level": "loud"}), want: expectValidation},
				accessCase{name: "malformed field type", who: "owner", body: staticBody(map[string]any{"jobs_worker_count": "many"}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			),
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "create instance account", method: http.MethodPost, path: instancePath("/accounts"), skipStandard: true,
			body: func(f *accessCharacterizationFixture) map[string]any {
				return map[string]any{"name": "Characterized Account", "email": uniqueEmail(t, "instance-char-new"), "password": "securepass99"}
			},
			extra: withInstanceCases(
				accessCase{name: "missing fields", who: "owner", body: staticBody(map[string]any{}), want: expectValidation},
				accessCase{name: "duplicate email", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Duplicate", "email": f.member.Email, "password": "securepass99"}
				}, want: expectValidation},
				accessCase{name: "short password", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Short", "email": uniqueEmail(t, "instance-char-short"), "password": "short"}
				}, want: expectValidation},
			),
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "add instance admin", method: http.MethodPost, path: instancePath("/admins"), skipStandard: true,
			body: func(f *accessCharacterizationFixture) map[string]any { return map[string]any{"email": f.member.Email} },
			extra: withInstanceCases(
				accessCase{name: "missing email", who: "owner", body: staticBody(map[string]any{}), want: expectValidation},
				accessCase{name: "unknown email", who: "owner", body: staticBody(map[string]any{"email": "nobody-here@example.com"}), want: expectNotFound},
			),
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "remove instance admin", method: http.MethodDelete, skipStandard: true,
			path: func(f *accessCharacterizationFixture) string {
				return "/api/v1/instance/admins/" + strconv.FormatInt(f.otherOwner.ID, 10)
			},
			extra: withInstanceCases(
				accessCase{name: "non-numeric account id", who: "owner", path: instancePath("/admins/abc"), want: expectNotFound},
			),
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "list account sessions", method: http.MethodGet, path: accountSessions, skipStandard: true,
			extra: withInstanceCases(badAccount("/sessions"), unknownAccount("/sessions")),
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "revoke all account sessions", method: http.MethodDelete, path: accountSessions, skipStandard: true,
			extra: withInstanceCases(badAccount("/sessions"), unknownAccount("/sessions")),
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "revoke one account session", method: http.MethodDelete, skipStandard: true,
			path: func(f *accessCharacterizationFixture) string { return accountSessions(f) + "/does-not-exist" },
			extra: withInstanceCases(
				badAccount("/sessions/x"), unknownAccount("/sessions/x"),
			),
			allow: expectNotFound,
		},
		{
			name: "rotate encryption keys", method: http.MethodPost, path: instancePath("/encryption/rotate"), skipStandard: true,
			extra: instanceRouteCases(),
			allow: expectStatus(http.StatusOK),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}

func TestInstanceLastAdminRemovalCharacterization(t *testing.T) {
	f := newAccessCharacterizationFixture(t)
	remove := func(accountID int64, token string) testResponse {
		return send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/instance/admins/"+strconv.FormatInt(accountID, 10), nil, token), f.app.routes())
	}
	if res := remove(f.otherOwner.ID, f.tokens["owner"]); res.StatusCode != http.StatusNoContent {
		t.Fatalf("removing a non-last admin = %d, want 204", res.StatusCode)
	}
	res := remove(f.owner.ID, f.tokens["owner"])
	if res.StatusCode != http.StatusUnprocessableEntity || res.ErrorCode() != apiErrorValidationFailed {
		t.Fatalf("removing the last admin = %d %q, want 422 %q", res.StatusCode, res.ErrorCode(), apiErrorValidationFailed)
	}
}

func TestSetupRoutesCharacterization(t *testing.T) {
	t.Run("status before setup is public", func(t *testing.T) {
		app := newTestApp(t)
		res := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
	})
	t.Run("status after setup is public", func(t *testing.T) {
		app := newTestApp(t)
		setupInstance(t, app, uniqueEmail(t, "setup-char"), "Setup Admin", "securepass99")
		res := send(t, newTestRequest(t, http.MethodGet, "/api/setup/status", nil), app.routes())
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
	})
	t.Run("setup twice conflicts", func(t *testing.T) {
		app := newTestApp(t)
		setupInstance(t, app, uniqueEmail(t, "setup-char-first"), "Setup Admin", "securepass99")
		res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{
			"email": uniqueEmail(t, "setup-char-second"), "name": "Second", "password": "securepass99", "organization_name": "Second Org",
		}), app.routes())
		if res.StatusCode != http.StatusConflict || res.ErrorCode() != apiErrorConflict {
			t.Fatalf("status = %d code = %q, want 409 %q", res.StatusCode, res.ErrorCode(), apiErrorConflict)
		}
	})
	t.Run("setup validation", func(t *testing.T) {
		app := newTestApp(t)
		res := send(t, newTestRequest(t, http.MethodPost, "/api/setup", map[string]any{}), app.routes())
		if res.StatusCode != http.StatusUnprocessableEntity || res.ErrorCode() != apiErrorValidationFailed {
			t.Fatalf("status = %d code = %q, want 422 %q", res.StatusCode, res.ErrorCode(), apiErrorValidationFailed)
		}
	})
}
