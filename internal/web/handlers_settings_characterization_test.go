package web

import (
	"net/http"
	"strconv"
	"testing"
)

func TestOrgRuntimeSettingsRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	settings := func(f *accessCharacterizationFixture) string { return f.orgPath() + "/runtime-settings" }

	routes := []accessRoute{
		{
			name: "get runtime settings", method: http.MethodGet, path: settings,
			member: &accessExpect{status: http.StatusOK},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "update runtime settings", method: http.MethodPatch, path: settings,
			body:   staticBody(map[string]any{"file_revisions_keep_latest": 5}),
			member: &notPermitted,
			extra: []accessCase{
				{name: "no settings", who: "owner", body: staticBody(map[string]any{}), want: expectValidation},
				{name: "unknown setting only", who: "owner", body: staticBody(map[string]any{"bogus": 1}), want: expectValidation},
				{name: "invalid history mode", who: "owner", body: staticBody(map[string]any{"query_history_mode": "bogus"}), want: expectValidation},
				{name: "value above instance limit", who: "owner", body: staticBody(map[string]any{"query_max_result_rows": 2147483647}), want: expectValidation},
				{name: "malformed field type", who: "owner", body: staticBody(map[string]any{"file_revisions_keep_latest": "many"}), want: accessExpect{http.StatusBadRequest, apiErrorBadRequest}},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "purge query history", method: http.MethodDelete,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/query-history" },
			member: &notPermitted,
			allow:  expectStatus(http.StatusNoContent),
		},
		{
			name: "purge query favorites", method: http.MethodDelete,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/query-favorites" },
			member: &notPermitted,
			allow:  expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}

func TestOrgMemberAccessSessionRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	sessions := func(f *accessCharacterizationFixture) string {
		return f.orgPath() + "/members/" + strconv.FormatInt(f.target.ID, 10) + "/sessions"
	}
	otherMemberSessions := func(f *accessCharacterizationFixture, id int64) string {
		return f.orgPath() + "/members/" + strconv.FormatInt(id, 10) + "/sessions"
	}

	routes := []accessRoute{
		{
			name: "list member access sessions", method: http.MethodGet, path: sessions,
			member: &notPermitted,
			extra: []accessCase{
				{name: "account outside org", who: "owner", path: func(f *accessCharacterizationFixture) string { return otherMemberSessions(f, f.nonMember.ID) }, want: expectNotFound},
				{name: "unknown account", who: "owner", path: func(f *accessCharacterizationFixture) string { return otherMemberSessions(f, 999999) }, want: expectNotFound},
				{name: "non-numeric account id", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/members/abc/sessions" }, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "revoke member access session", method: http.MethodDelete,
			path:   func(f *accessCharacterizationFixture) string { return sessions(f) + "/does-not-exist" },
			member: &notPermitted,
			extra: []accessCase{
				{name: "account outside org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return otherMemberSessions(f, f.nonMember.ID) + "/does-not-exist"
				}, want: expectNotFound},
				{name: "non-numeric account id", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/members/abc/sessions/x" }, want: expectNotFound},
			},
			allow: expectNotFound,
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}
