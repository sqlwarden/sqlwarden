package web

import (
	"net/http"
	"strconv"
	"testing"
)

func TestTeamRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	teamPath := func(suffix string) func(*accessCharacterizationFixture) string {
		return func(f *accessCharacterizationFixture) string {
			return f.orgPath() + "/teams/" + f.team.Slug + suffix
		}
	}
	foreignTeam := func(suffix string) accessCase {
		return accessCase{name: "team of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
			return f.orgPath() + "/teams/" + f.foreignTeam.Slug + suffix
		}, want: expectNotFound}
	}
	nameBody := func(*accessCharacterizationFixture) map[string]any { return map[string]any{"name": "Renamed Team"} }
	accountBody := func(f *accessCharacterizationFixture) map[string]any {
		return map[string]any{"account_id": f.target.ID}
	}

	routes := []accessRoute{
		{
			name: "list teams", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/teams" },
			member: &accessExpect{status: http.StatusOK},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "create team", method: http.MethodPost,
			path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/teams" },
			body: func(*accessCharacterizationFixture) map[string]any {
				return map[string]any{"slug": "new-team", "name": "New Team"}
			},
			member: &notPermitted,
			extra: []accessCase{
				{name: "duplicate slug", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"slug": "char-team", "name": "Duplicate"}
				}, want: expectValidation},
				{name: "missing fields", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "get team", method: http.MethodGet,
			path:   teamPath(""),
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				foreignTeam(""),
				{name: "unknown team", who: "owner", path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/teams/missing" }, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "update team", method: http.MethodPatch,
			path:   teamPath(""),
			body:   nameBody,
			member: &notPermitted,
			extra:  []accessCase{foreignTeam("")},
			allow:  expectStatus(http.StatusNoContent),
		},
		{
			name: "delete team", method: http.MethodDelete,
			path:   teamPath(""),
			member: &notPermitted,
			extra:  []accessCase{foreignTeam("")},
			allow:  expectStatus(http.StatusNoContent),
		},
		{
			name: "list team members", method: http.MethodGet,
			path:   teamPath("/members"),
			member: &accessExpect{status: http.StatusOK},
			extra:  []accessCase{foreignTeam("/members")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "add team member", method: http.MethodPost,
			path:   teamPath("/members"),
			body:   accountBody,
			member: &notPermitted,
			extra: []accessCase{
				foreignTeam("/members"),
				{name: "account outside org", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return map[string]any{"account_id": f.nonMember.ID}
				}, want: expectNotFound},
				{name: "missing account", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "remove team member", method: http.MethodDelete,
			path: func(f *accessCharacterizationFixture) string {
				return f.orgPath() + "/teams/" + f.team.Slug + "/members/" + strconv.FormatInt(f.target.ID, 10)
			},
			member: &notPermitted,
			extra: []accessCase{
				{name: "team of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/teams/" + f.foreignTeam.Slug + "/members/" + strconv.FormatInt(f.target.ID, 10)
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}
