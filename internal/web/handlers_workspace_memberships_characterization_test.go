package web

import (
	"net/http"
	"strconv"
	"testing"
)

func TestWorkspaceMembershipRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	users := func(suffix string) func(*accessCharacterizationFixture) string {
		return func(f *accessCharacterizationFixture) string { return f.wsPath() + "/users" + suffix }
	}
	teams := func(suffix string) func(*accessCharacterizationFixture) string {
		return func(f *accessCharacterizationFixture) string { return f.wsPath() + "/teams" + suffix }
	}
	foreignWorkspace := func(suffix string) accessCase {
		return accessCase{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
			return f.foreignWSPath() + suffix
		}, want: expectNotFound}
	}
	workspaceMemberDenied := accessCase{name: "workspace member", who: "wsMember", want: expectNotPermitted}
	accountBody := func(f *accessCharacterizationFixture) map[string]any {
		return map[string]any{"account_id": f.target.ID}
	}
	teamBody := func(f *accessCharacterizationFixture) map[string]any {
		return map[string]any{"team_id": f.team.ID}
	}

	routes := []accessRoute{
		{
			name: "list workspace members", method: http.MethodGet,
			path:   users(""),
			member: &notPermitted,
			extra:  []accessCase{workspaceMemberDenied, foreignWorkspace("/users")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "list workspace effective members", method: http.MethodGet,
			path:   users("/effective"),
			member: &notPermitted,
			extra:  []accessCase{workspaceMemberDenied, foreignWorkspace("/users/effective")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "add workspace member", method: http.MethodPost,
			path:   users(""),
			body:   accountBody,
			member: &notPermitted,
			extra: []accessCase{
				workspaceMemberDenied, foreignWorkspace("/users"),
				{name: "account outside org", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return map[string]any{"account_id": f.nonMember.ID}
				}, want: expectNotFound},
				{name: "missing account", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "remove workspace member", method: http.MethodDelete,
			path: func(f *accessCharacterizationFixture) string {
				return f.wsPath() + "/users/" + strconv.FormatInt(f.wsMember.ID, 10)
			},
			member: &notPermitted,
			extra: []accessCase{
				workspaceMemberDenied,
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.foreignWSPath() + "/users/" + strconv.FormatInt(f.wsMember.ID, 10)
				}, want: expectNotFound},
				{name: "malformed account id", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.wsPath() + "/users/abc"
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "list workspace teams", method: http.MethodGet,
			path:   teams(""),
			member: &notPermitted,
			extra:  []accessCase{workspaceMemberDenied, foreignWorkspace("/teams")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "add workspace team", method: http.MethodPost,
			path:   teams(""),
			body:   teamBody,
			member: &notPermitted,
			extra: []accessCase{
				workspaceMemberDenied, foreignWorkspace("/teams"),
				{name: "team of another org", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return map[string]any{"team_id": f.foreignTeam.ID}
				}, want: expectNotFound},
				{name: "missing team", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "remove workspace team", method: http.MethodDelete,
			path: func(f *accessCharacterizationFixture) string {
				return f.wsPath() + "/teams/" + strconv.FormatInt(f.team.ID, 10)
			},
			member: &notPermitted,
			extra: []accessCase{
				workspaceMemberDenied,
				{name: "workspace of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.foreignWSPath() + "/teams/" + strconv.FormatInt(f.team.ID, 10)
				}, want: expectNotFound},
				{name: "malformed team id", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.wsPath() + "/teams/abc"
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newAccessCharacterizationFixture, routes)
}
