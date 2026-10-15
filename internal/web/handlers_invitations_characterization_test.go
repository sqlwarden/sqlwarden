package web

import (
	"net/http"
	"testing"
)

func newInvitationCharacterizationFixture(t *testing.T) *accessCharacterizationFixture {
	return newAccessCharacterizationFixture(t).withInvitations(t)
}

func TestOrgInvitationRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	invitationPath := func(suffix string) func(*accessCharacterizationFixture) string {
		return func(f *accessCharacterizationFixture) string {
			return f.orgPath() + "/invitations/" + f.invitationID + suffix
		}
	}
	foreignInvitation := func(suffix string) accessCase {
		return accessCase{name: "invitation of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
			return f.orgPath() + "/invitations/" + f.foreignInvID + suffix
		}, want: expectNotFound}
	}

	routes := []accessRoute{
		{
			name: "list invitations", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/invitations" },
			member: &notPermitted,
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "create invitation", method: http.MethodPost,
			path: func(f *accessCharacterizationFixture) string { return f.orgPath() + "/invitations" },
			body: func(*accessCharacterizationFixture) map[string]any {
				return map[string]any{"email": "characterized-new-invitee@example.com"}
			},
			member: &notPermitted,
			extra: []accessCase{
				{name: "invalid email", who: "owner", body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"email": "not-an-email"}
				}, want: expectValidation},
				{name: "already a member", who: "owner", body: func(f *accessCharacterizationFixture) map[string]any {
					return map[string]any{"email": f.member.Email}
				}, want: expectValidation},
			},
			allow: expectStatus(http.StatusCreated),
		},
		{
			name: "resend invitation", method: http.MethodPost,
			path:   invitationPath("/resend"),
			member: &notPermitted,
			extra:  []accessCase{foreignInvitation("/resend")},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "revoke invitation", method: http.MethodDelete,
			path:   invitationPath(""),
			member: &notPermitted,
			extra:  []accessCase{foreignInvitation("")},
			allow:  expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newInvitationCharacterizationFixture, routes)
}

func TestPublicInvitationRoutesCharacterization(t *testing.T) {
	existingToken := func(f *accessCharacterizationFixture) string { return "/api/v1/invitations/" + f.inviteToken }
	newcomerToken := func(f *accessCharacterizationFixture) string { return "/api/v1/invitations/" + f.newInviteTok }
	unknownToken := func(*accessCharacterizationFixture) string { return "/api/v1/invitations/unknown-token" }
	foreignToken := func(f *accessCharacterizationFixture) string { return "/api/v1/invitations/" + f.foreignInvTok }
	acceptExisting := func(f *accessCharacterizationFixture) string { return existingToken(f) + "/accept" }
	acceptNewcomer := func(f *accessCharacterizationFixture) string { return newcomerToken(f) + "/accept" }
	acceptUnknown := func(f *accessCharacterizationFixture) string { return unknownToken(f) + "/accept" }
	acceptForeign := func(f *accessCharacterizationFixture) string { return foreignToken(f) + "/accept" }
	signUpBody := func(*accessCharacterizationFixture) map[string]any {
		return map[string]any{"name": "Newcomer", "password": "newcomer-password"}
	}

	routes := []accessRoute{
		{
			name: "get invitation", method: http.MethodGet, skipStandard: true,
			path: existingToken,
			extra: []accessCase{
				{name: "no authorization header", who: "anonymous", want: expectStatus(http.StatusOK)},
				{name: "empty bearer token", who: "", want: expectInvalidToken},
				{name: "other authenticated account", who: "member", want: expectStatus(http.StatusOK)},
				{name: "invitation of another org", who: "member", path: foreignToken, want: expectStatus(http.StatusOK)},
				{name: "unknown token", who: "owner", path: unknownToken, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "accept invitation", method: http.MethodPost, skipStandard: true,
			path: acceptExisting,
			body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} },
			extra: []accessCase{
				{name: "no authorization header for existing account", who: "anonymous", want: accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}},
				{name: "empty bearer token", who: "", want: expectInvalidToken},
				{name: "wrong account", who: "member", want: expectNotPermitted},
				{name: "unknown token", who: "invitee", path: acceptUnknown, want: expectNotFound},
				{name: "signed in while invited account does not exist", who: "member", path: acceptNewcomer, body: signUpBody, want: accessExpect{http.StatusConflict, apiErrorConflict}},
				{name: "invitation of another org while signed in", who: "invitee", path: acceptForeign, body: signUpBody, want: accessExpect{http.StatusConflict, apiErrorConflict}},
				{name: "anonymous sign up with short password", who: "anonymous", path: acceptNewcomer, body: func(*accessCharacterizationFixture) map[string]any {
					return map[string]any{"name": "Newcomer", "password": "short"}
				}, want: expectValidation},
				{name: "anonymous sign up", who: "anonymous", path: acceptNewcomer, body: signUpBody, want: expectStatus(http.StatusCreated)},
			},
			allowWho: "invitee",
			allow:    expectStatus(http.StatusOK),
		},
	}
	runAccessCharacterization(t, newInvitationCharacterizationFixture, routes)
}
