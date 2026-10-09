package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/database"
)

type connectionCharacterizationFixture struct {
	app             *application
	ownerToken      string
	memberToken     string
	nonMemberToken  string
	org             database.Organization
	foreignOrg      database.Organization
	workspace       database.Workspace
	environmentID   int64
	environmentConn database.Connection
	personalConn    database.Connection
}

type connectionCharacterizationRoute struct {
	name                string
	method              string
	template            string
	path                func(connectionCharacterizationFixture, string) string
	body                map[string]any
	wantAllowedStatus   int
	wantMemberStatus    int
	wantMemberErrorCode string
}

var connectionCharacterizationRoutes = []connectionCharacterizationRoute{
	{
		name: "organization environment create", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgEnvConnectionsURL(orgSlug, f.workspace.ID, f.environmentID)
		},
		body: map[string]any{
			"name": "Environment Created", "driver": "sqlite", "params": map[string]any{"path": ":memory:"},
		},
		wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "organization environment get", method: http.MethodGet,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgConnectionURL(orgSlug, f.workspace.ID, f.environmentID, strconv.FormatInt(f.environmentConn.ID, 10))
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberErrorCode: apiErrorNotFound,
	},
	{
		name: "organization environment list", method: http.MethodGet,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgEnvConnectionsURL(orgSlug, f.workspace.ID, f.environmentID)
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK,
	},
	{
		name: "organization environment update", method: http.MethodPatch,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgConnectionURL(orgSlug, f.workspace.ID, f.environmentID, strconv.FormatInt(f.environmentConn.ID, 10))
		},
		body:              map[string]any{"name": "Environment Updated"},
		wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "organization environment test", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections/test",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgEnvConnectionsURL(orgSlug, f.workspace.ID, f.environmentID) + "/test"
		},
		body:              map[string]any{"driver": "sqlite", "params": map[string]any{"path": ":memory:"}},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "organization environment connect", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections/{conn_id}/connect",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgConnectionURL(orgSlug, f.workspace.ID, f.environmentID, strconv.FormatInt(f.environmentConn.ID, 10)) + "/connect"
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "organization environment delete", method: http.MethodDelete,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return orgConnectionURL(orgSlug, f.workspace.ID, f.environmentID, strconv.FormatInt(f.environmentConn.ID, 10))
		},
		wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "personal space create", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections", orgSlug, f.workspace.ID)
		},
		body: map[string]any{
			"name": "Personal Created", "driver": "sqlite", "params": map[string]any{"path": ":memory:"},
		},
		wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "personal space get", method: http.MethodGet,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%d", orgSlug, f.workspace.ID, f.personalConn.ID)
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberErrorCode: apiErrorNotFound,
	},
	{
		name: "personal space list", method: http.MethodGet,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections", orgSlug, f.workspace.ID)
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK,
	},
	{
		name: "personal space update", method: http.MethodPatch,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%d", orgSlug, f.workspace.ID, f.personalConn.ID)
		},
		body:              map[string]any{"name": "Personal Updated"},
		wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "personal space test", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/test",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/test", orgSlug, f.workspace.ID)
		},
		body:              map[string]any{"driver": "sqlite", "params": map[string]any{"path": ":memory:"}},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "personal space connect", method: http.MethodPost,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}/connect",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%d/connect", orgSlug, f.workspace.ID, f.personalConn.ID)
		},
		wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
	{
		name: "personal space delete", method: http.MethodDelete,
		template: "/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}",
		path: func(f connectionCharacterizationFixture, orgSlug string) string {
			return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/connections/%d", orgSlug, f.workspace.ID, f.personalConn.ID)
		},
		wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberErrorCode: apiErrorNotPermitted,
	},
}

func TestConnectionRoutesCharacterization(t *testing.T) {
	fixture := newConnectionCharacterizationFixture(t)

	for _, route := range connectionCharacterizationRoutes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			path := route.path(fixture, fixture.org.Slug)
			cases := []struct {
				name       string
				token      string
				path       string
				wantStatus int
				wantCode   string
			}{
				{name: "unauthenticated", path: path, wantStatus: http.StatusUnauthorized, wantCode: apiErrorInvalidAuthenticationToken},
				{name: "non-member", token: fixture.nonMemberToken, path: path, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "member without connection permissions", token: fixture.memberToken, path: path, wantStatus: route.wantMemberStatus, wantCode: route.wantMemberErrorCode},
				{name: "foreign workspace", token: fixture.ownerToken, path: route.path(fixture, fixture.foreignOrg.Slug), wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound},
				{name: "allowed", token: fixture.ownerToken, path: path, wantStatus: route.wantAllowedStatus},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					res := send(t, newAuthRequest(t, route.method, tc.path, route.body, tc.token), fixture.app.routes())
					if res.StatusCode != tc.wantStatus {
						t.Fatalf("%s %s: status = %d, want %d", route.method, route.template, res.StatusCode, tc.wantStatus)
					}
					if got := res.ErrorCode(); got != tc.wantCode {
						t.Fatalf("%s %s: error code = %q, want %q", route.method, route.template, got, tc.wantCode)
					}
				})
			}
		})
	}
}

func newConnectionCharacterizationFixture(t *testing.T) connectionCharacterizationFixture {
	t.Helper()

	app := newTestApp(t)
	owner, ownerToken, org := seedOrgOwner(t, app, uniqueEmail(t, "connection-characterization-owner"), "Connection Characterization Owner", "Connection Characterization")
	member, memberToken := seedAccountWithToken(t, app, uniqueEmail(t, "connection-characterization-member"), "Connection Characterization Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	_, nonMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "connection-characterization-non-member"), "Connection Characterization Non-member")

	workspace := seedWorkspaceForAccount(t, app, org, owner, "Characterization Workspace", "")
	environmentID := defaultEnvironmentID(t, app, workspace.ID)
	foreignOrg := seedOrganizationForAccount(t, app, owner, "Foreign Characterization Organization")

	environmentConn := insertStructuredConnection(t, app, workspace.ID, &environmentID, "Environment Characterization", "sqlite", map[string]any{"path": ":memory:"})
	personalConn := insertStructuredConnection(t, app, workspace.ID, nil, "Personal Characterization", "sqlite", map[string]any{"path": ":memory:"})

	return connectionCharacterizationFixture{
		app: app, ownerToken: ownerToken, memberToken: memberToken, nonMemberToken: nonMemberToken,
		org: org, foreignOrg: foreignOrg, workspace: workspace, environmentID: environmentID,
		environmentConn: environmentConn, personalConn: personalConn,
	}
}
