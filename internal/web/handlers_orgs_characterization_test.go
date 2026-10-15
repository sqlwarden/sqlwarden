package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/database"
)

type accessCharacterizationFixture struct {
	app *application

	org           database.Organization
	owner         database.Account
	member        database.Account
	wsMember      database.Account
	target        database.Account
	nonMember     database.Account
	otherOwner    database.Account
	otherOrg      database.Organization
	workspace     database.Workspace
	spareEnvID    int64
	defaultEnvID  int64
	foreignWS     database.Workspace
	foreignEnvID  int64
	team          database.Team
	foreignTeam   database.Team
	tokens        map[string]string
	invitationID  string
	foreignInvID  string
	inviteToken   string
	newInviteTok  string
	inviteeEmail  string
	foreignInvTok string
}

type accessExpect struct {
	status int
	code   string
}

type accessCase struct {
	name string
	who  string
	path func(*accessCharacterizationFixture) string
	body func(*accessCharacterizationFixture) map[string]any
	want accessExpect
}

type accessRoute struct {
	name         string
	method       string
	path         func(*accessCharacterizationFixture) string
	body         func(*accessCharacterizationFixture) map[string]any
	skipStandard bool
	allowWho     string
	member       *accessExpect
	extra        []accessCase
	allow        accessExpect
}

var (
	expectNotPermitted = accessExpect{http.StatusForbidden, apiErrorNotPermitted}
	expectNotFound     = accessExpect{http.StatusNotFound, apiErrorNotFound}
	expectInvalidToken = accessExpect{http.StatusUnauthorized, apiErrorInvalidAuthenticationToken}
	expectValidation   = accessExpect{http.StatusUnprocessableEntity, apiErrorValidationFailed}
	expectStatus       = func(status int) accessExpect { return accessExpect{status: status} }
)

func newAccessCharacterizationFixture(t *testing.T) *accessCharacterizationFixture {
	t.Helper()
	ctx := context.Background()
	app := newTestApp(t)
	f := &accessCharacterizationFixture{app: app, tokens: map[string]string{}}

	var ownerToken string
	f.owner, ownerToken, f.org = seedOrgOwner(t, app, uniqueEmail(t, "access-char-owner"), "Access Characterization Owner", "Access Characterization")
	f.tokens["owner"] = ownerToken

	addMember := func(key, prefix string) database.Account {
		account, token := seedAccountWithToken(t, app, uniqueEmail(t, prefix), prefix)
		if err := testDB(app).AddOrgMember(ctx, f.org.ID, account.ID); err != nil {
			t.Fatal(err)
		}
		f.tokens[key] = token
		return account
	}
	f.member = addMember("member", "access-char-member")
	f.wsMember = addMember("wsMember", "access-char-ws-member")
	f.target = addMember("target", "access-char-target")

	var nonMemberToken string
	f.nonMember, nonMemberToken = seedAccountWithToken(t, app, uniqueEmail(t, "access-char-non-member"), "Access Characterization Non-member")
	f.tokens["nonMember"] = nonMemberToken

	var otherToken string
	f.otherOwner, otherToken, f.otherOrg = seedOrgOwner(t, app, uniqueEmail(t, "access-char-other-owner"), "Access Characterization Other Owner", "Access Characterization Other")
	f.tokens["otherOwner"] = otherToken

	f.workspace = seedWorkspaceForAccount(t, app, f.org, f.owner, "Access Characterization Workspace", "")
	f.defaultEnvID = defaultEnvironmentID(t, app, f.workspace.ID)
	spare, err := testDB(app).InsertEnvironment(ctx, f.workspace.ID, "Spare", "")
	if err != nil {
		t.Fatal(err)
	}
	f.spareEnvID = spare.ID
	if err := testDB(app).AddWorkspaceMember(ctx, f.workspace.ID, f.wsMember.ID, nil); err != nil {
		t.Fatal(err)
	}
	app.enforcer.InvalidatePrincipals(f.org.ID, f.wsMember.ID)

	f.foreignWS = seedWorkspaceForAccount(t, app, f.otherOrg, f.otherOwner, "Foreign Access Characterization Workspace", "")
	f.foreignEnvID = defaultEnvironmentID(t, app, f.foreignWS.ID)

	if f.team, err = testDB(app).InsertTeam(ctx, f.org.ID, "char-team", "Characterization Team"); err != nil {
		t.Fatal(err)
	}
	if f.foreignTeam, err = testDB(app).InsertTeam(ctx, f.otherOrg.ID, "foreign-team", "Foreign Team"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *accessCharacterizationFixture) withInvitations(t *testing.T) *accessCharacterizationFixture {
	t.Helper()
	invitee, inviteeToken := seedAccountWithToken(t, f.app, uniqueEmail(t, "access-char-invitee"), "Access Characterization Invitee")
	f.tokens["invitee"] = inviteeToken
	f.inviteeEmail = invitee.Email
	f.invitationID, f.inviteToken = createCharacterizationInvitation(t, f.app, f.org.Slug, f.tokens["owner"], invitee.Email)
	_, f.newInviteTok = createCharacterizationInvitation(t, f.app, f.org.Slug, f.tokens["owner"], uniqueEmail(t, "access-char-newcomer"))
	f.foreignInvID, f.foreignInvTok = createCharacterizationInvitation(t, f.app, f.otherOrg.Slug, f.tokens["otherOwner"], uniqueEmail(t, "access-char-foreign-invitee"))
	return f
}

func createCharacterizationInvitation(t *testing.T, app *application, orgSlug, token, email string) (string, string) {
	t.Helper()
	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+orgSlug+"/invitations", map[string]any{"email": email}, token), app.routes())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create invitation: status = %d, body %s", res.StatusCode, res.BodyBytes)
	}
	invitation, _ := res.BodyFields["invitation"].(map[string]any)
	inviteURL, _ := res.BodyFields["invite_url"].(string)
	for i := len(inviteURL) - 1; i >= 0; i-- {
		if inviteURL[i] == '/' {
			return fmt.Sprint(invitation["id"]), inviteURL[i+1:]
		}
	}
	t.Fatalf("invite url %q has no token", inviteURL)
	return "", ""
}

func (f *accessCharacterizationFixture) orgPath() string {
	return "/api/v1/orgs/" + f.org.Slug
}

func (f *accessCharacterizationFixture) wsPath() string {
	return f.orgPath() + "/workspaces/" + strconv.FormatInt(f.workspace.ID, 10)
}

func (f *accessCharacterizationFixture) foreignWSPath() string {
	return f.orgPath() + "/workspaces/" + strconv.FormatInt(f.foreignWS.ID, 10)
}

func runAccessCharacterization(t *testing.T, setup func(*testing.T) *accessCharacterizationFixture, routes []accessRoute) {
	t.Helper()
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			cases := []accessCase{}
			if !route.skipStandard {
				cases = append(cases,
					accessCase{name: "empty bearer token", who: "", want: expectInvalidToken},
					accessCase{name: "no authorization header", who: "anonymous", want: accessExpect{http.StatusUnauthorized, apiErrorAuthenticationRequired}},
					accessCase{name: "non-member of org", who: "nonMember", want: expectNotPermitted},
					accessCase{name: "member of another org", who: "otherOwner", want: expectNotPermitted},
				)
				if route.member != nil {
					cases = append(cases, accessCase{name: "org member without grants", who: "member", want: *route.member})
				}
			}
			cases = append(cases, route.extra...)
			allowWho := route.allowWho
			if allowWho == "" {
				allowWho = "owner"
			}
			cases = append(cases, accessCase{name: "allowed", who: allowWho, want: route.allow})

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					f := setup(t)
					path := route.path(f)
					if tc.path != nil {
						path = tc.path(f)
					}
					body := route.body
					if tc.body != nil {
						body = tc.body
					}
					var payload map[string]any
					if body != nil {
						payload = body(f)
					}
					var req *http.Request
					if tc.who == "anonymous" {
						req = newTestRequest(t, route.method, path, payload)
					} else {
						req = newAuthRequest(t, route.method, path, payload, f.tokens[tc.who])
					}
					res := send(t, req, f.app.routes())
					if res.StatusCode != tc.want.status {
						t.Fatalf("status = %d, want %d, body %s", res.StatusCode, tc.want.status, res.BodyBytes)
					}
					if got := res.ErrorCode(); got != tc.want.code {
						t.Fatalf("error code = %q, want %q", got, tc.want.code)
					}
				})
			}
		})
	}
}

func newOrgCharacterizationFixture(t *testing.T) *accessCharacterizationFixture {
	return newAccessCharacterizationFixture(t)
}

func memberPath(suffix string) func(*accessCharacterizationFixture) string {
	return func(f *accessCharacterizationFixture) string {
		return f.orgPath() + "/members/" + strconv.FormatInt(f.target.ID, 10) + suffix
	}
}

func TestOrgRoutesCharacterization(t *testing.T) {
	notPermitted := expectNotPermitted
	routes := []accessRoute{
		{
			name: "get org", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() },
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "unknown org slug", who: "owner", path: func(*accessCharacterizationFixture) string { return "/api/v1/orgs/no-such-org" }, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "update org", method: http.MethodPatch,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() },
			body:   func(*accessCharacterizationFixture) map[string]any { return map[string]any{"name": "Renamed Org"} },
			member: &notPermitted,
			extra: []accessCase{
				{name: "empty body", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "delete org", method: http.MethodDelete,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() },
			member: &notPermitted,
			allow:  expectStatus(http.StatusNoContent),
		},
		{
			name: "list members", method: http.MethodGet,
			path:   func(f *accessCharacterizationFixture) string { return f.orgPath() + "/members" },
			member: &accessExpect{status: http.StatusOK},
			allow:  expectStatus(http.StatusOK),
		},
		{
			name: "get member", method: http.MethodGet,
			path:   memberPath(""),
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "account outside org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.nonMember.ID, 10)
				}, want: expectNotFound},
				{name: "account of another org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.otherOwner.ID, 10)
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "list member teams", method: http.MethodGet,
			path:   memberPath("/teams"),
			member: &accessExpect{status: http.StatusOK},
			extra: []accessCase{
				{name: "account outside org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.nonMember.ID, 10) + "/teams"
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusOK),
		},
		{
			name: "update member role", method: http.MethodPatch,
			path:   memberPath(""),
			body:   func(*accessCharacterizationFixture) map[string]any { return map[string]any{"role": "Administrator"} },
			member: &notPermitted,
			extra: []accessCase{
				{name: "last owner demotion", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.owner.ID, 10)
				}, body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{"role": "Baseline Access"} }, want: expectValidation},
				{name: "invalid role", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{"role": "Nope"} }, want: expectValidation},
				{name: "account outside org", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.nonMember.ID, 10)
				}, want: expectNotFound},
			},
			allow: expectStatus(http.StatusNoContent),
		},
		{
			name: "remove member", method: http.MethodDelete,
			path:   memberPath(""),
			member: &notPermitted,
			extra: []accessCase{
				{name: "last owner removal", who: "owner", path: func(f *accessCharacterizationFixture) string {
					return f.orgPath() + "/members/" + strconv.FormatInt(f.owner.ID, 10)
				}, want: expectValidation},
			},
			allow: expectStatus(http.StatusNoContent),
		},
	}
	runAccessCharacterization(t, newOrgCharacterizationFixture, routes)
}

func TestCreateOrgRouteCharacterization(t *testing.T) {
	routes := []accessRoute{{
		name: "create org", method: http.MethodPost, skipStandard: true,
		path: func(*accessCharacterizationFixture) string { return "/api/v1/orgs" },
		body: func(*accessCharacterizationFixture) map[string]any {
			return map[string]any{"name": "Characterized New Org"}
		},
		extra: []accessCase{
			{name: "unauthenticated", who: "", want: expectInvalidToken},
			{name: "account that is not an instance admin", who: "member", want: expectNotPermitted},
			{name: "missing name", who: "owner", body: func(*accessCharacterizationFixture) map[string]any { return map[string]any{} }, want: expectValidation},
		},
		allow: expectStatus(http.StatusCreated),
	}}
	runAccessCharacterization(t, newOrgCharacterizationFixture, routes)
}
