package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
)

type exportsCharacterizationFixture struct {
	app                    *application
	ownerToken             string
	memberToken            string
	nonMemberToken         string
	differentConnToken     string
	targetURL              string
	foreignConnectionURL   string
	ownerSessionID         string
	differentConnSessionID string
}

func TestExportRoutesCharacterization(t *testing.T) {
	routes := []struct {
		name              string
		suffix            string
		usesSession       bool
		wantAllowedStatus int
	}{
		{name: "create export", suffix: "/exports", wantAllowedStatus: http.StatusCreated},
		{name: "download export", suffix: "/exports/download", usesSession: true, wantAllowedStatus: http.StatusOK},
	}
	for _, route := range routes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			fixture := newExportsCharacterizationFixture(t)
			cases := []struct {
				name       string
				token      string
				baseURL    string
				sessionID  string
				wantStatus int
				wantCode   string
			}{
				{name: "unauthenticated", baseURL: fixture.targetURL, wantStatus: http.StatusUnauthorized, wantCode: apiErrorInvalidAuthenticationToken},
				{name: "non-member", token: fixture.nonMemberToken, baseURL: fixture.targetURL, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "org member without workspace access", token: fixture.memberToken, baseURL: fixture.targetURL, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "permission on different connection", token: fixture.differentConnToken, baseURL: fixture.targetURL, sessionID: fixture.differentConnSessionID, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "connection from another workspace", token: fixture.ownerToken, baseURL: fixture.foreignConnectionURL, sessionID: fixture.ownerSessionID, wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound},
				{name: "allowed", token: fixture.ownerToken, baseURL: fixture.targetURL, sessionID: fixture.ownerSessionID, wantStatus: route.wantAllowedStatus},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					req := newAuthRequest(t, http.MethodPost, tc.baseURL+route.suffix, map[string]any{
						"sql": "SELECT 1 AS characterized", "format": "csv", "filename": "characterized",
					}, tc.token)
					if route.usesSession {
						req.Header.Set("X-Warden-Session", tc.sessionID)
					}
					res := send(t, req, fixture.app.routes())
					if res.StatusCode != tc.wantStatus {
						t.Fatalf("status = %d, want %d", res.StatusCode, tc.wantStatus)
					}
					if got := res.ErrorCode(); got != tc.wantCode {
						t.Fatalf("error code = %q, want %q", got, tc.wantCode)
					}
				})
			}
		})
	}
}

func newExportsCharacterizationFixture(t *testing.T) exportsCharacterizationFixture {
	t.Helper()
	app := newTestApp(t)
	owner, ownerToken, org := seedOrgOwner(t, app, uniqueEmail(t, "exports-char-owner"), "Exports Characterization Owner", "Exports Characterization")
	member, memberToken := seedAccountWithToken(t, app, uniqueEmail(t, "exports-char-member"), "Exports Characterization Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	different, differentToken := seedAccountWithToken(t, app, uniqueEmail(t, "exports-char-different"), "Exports Characterization Different")
	if err := app.db.AddOrgMember(context.Background(), org.ID, different.ID); err != nil {
		t.Fatal(err)
	}
	_, nonMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "exports-char-non-member"), "Exports Characterization Non-member")

	workspace := seedWorkspaceForAccount(t, app, org, owner, "Exports Characterization Workspace", "")
	environmentID := defaultEnvironmentID(t, app, workspace.ID)
	target := insertStructuredConnection(t, app, workspace.ID, &environmentID, "Exports Characterization Target", "sqlite", map[string]any{"path": ":memory:"})
	differentConn := insertStructuredConnection(t, app, workspace.ID, &environmentID, "Exports Characterization Different", "sqlite", map[string]any{"path": ":memory:"})
	foreignWorkspace := seedWorkspaceForAccount(t, app, org, owner, "Exports Characterization Foreign", "")
	foreignEnvironmentID := defaultEnvironmentID(t, app, foreignWorkspace.ID)
	foreignConn := insertStructuredConnection(t, app, foreignWorkspace.ID, &foreignEnvironmentID, "Exports Characterization Foreign", "sqlite", map[string]any{"path": ":memory:"})

	roleID := createRoleForTest(t, app, org.ID, nil, "connection", access.PermConnRead, access.PermConnExecute, access.PermConnDQL)
	grant := grantWorkspacePolicyRole(t, app, ownerToken, org.Slug, strconv.FormatInt(workspace.ID, 10), roleID, access.SubjectTypeAccount, different.ID, "connection", differentConn.ID)
	if grant.StatusCode != http.StatusNoContent {
		t.Fatalf("grant status = %d", grant.StatusCode)
	}

	targetURL := orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(target.ID, 10))
	differentURL := orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(differentConn.ID, 10))
	return exportsCharacterizationFixture{
		app: app, ownerToken: ownerToken, memberToken: memberToken, nonMemberToken: nonMemberToken, differentConnToken: differentToken,
		targetURL:              targetURL,
		foreignConnectionURL:   orgConnectionURL(org.Slug, workspace.ID, environmentID, strconv.FormatInt(foreignConn.ID, 10)),
		ownerSessionID:         connectCharacterizationSession(t, app, targetURL, ownerToken),
		differentConnSessionID: connectCharacterizationSession(t, app, differentURL, differentToken),
	}
}
