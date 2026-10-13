package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/database"
)

type filesCharacterizationFixture struct {
	app                  *application
	ownerToken           string
	memberToken          string
	workspaceMemberToken string
	nonMemberToken       string
	org                  database.Organization
	workspace            database.Workspace
	privateFileID        int64
	sharedFileID         int64
	foreignPrivateID     int64
	privateETag          string
	sharedETag           string
}

type filesCharacterizationRoute struct {
	name              string
	method            string
	visibility        string
	suffix            func(filesCharacterizationFixture, int64) string
	body              map[string]any
	contentRequest    bool
	itemRoute         bool
	wantAllowedStatus int
	wantMemberStatus  int
	wantMemberCode    string
}

var filesCharacterizationRoutes = []filesCharacterizationRoute{
	{name: "private list", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: filesCollectionSuffix, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK},
	{name: "private create", method: http.MethodPost, visibility: database.FileVisibilityPrivate, suffix: filesCollectionSuffix, body: map[string]any{"name": "created.sql"}, wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusCreated},
	{name: "private browser", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: func(filesCharacterizationFixture, int64) string { return "/browser" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK},
	{name: "private recent", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: func(filesCharacterizationFixture, int64) string { return "/recent?limit=5" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK},
	{name: "private search", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: func(filesCharacterizationFixture, int64) string { return "/search?q=select" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusOK},
	{name: "private get", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: filesItemSuffix, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "private patch", method: http.MethodPatch, visibility: database.FileVisibilityPrivate, suffix: filesItemSuffix, body: map[string]any{"name": "renamed.sql"}, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "private duplicate", method: http.MethodPost, visibility: database.FileVisibilityPrivate, suffix: func(_ filesCharacterizationFixture, id int64) string {
		return "/" + strconv.FormatInt(id, 10) + "/duplicate"
	}, body: map[string]any{"name": "copy.sql"}, itemRoute: true, wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "private delete", method: http.MethodDelete, visibility: database.FileVisibilityPrivate, suffix: filesItemSuffix, itemRoute: true, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "private content get", method: http.MethodGet, visibility: database.FileVisibilityPrivate, suffix: filesContentSuffix, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "private content put", method: http.MethodPut, visibility: database.FileVisibilityPrivate, suffix: filesContentSuffix, contentRequest: true, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusNotFound, wantMemberCode: apiErrorNotFound},
	{name: "shared list", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: filesCollectionSuffix, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared create", method: http.MethodPost, visibility: database.FileVisibilityShared, suffix: filesCollectionSuffix, body: map[string]any{"name": "created.sql"}, wantAllowedStatus: http.StatusCreated, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared browser", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: func(filesCharacterizationFixture, int64) string { return "/browser" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared recent", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: func(filesCharacterizationFixture, int64) string { return "/recent?limit=5" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared search", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: func(filesCharacterizationFixture, int64) string { return "/search?q=select" }, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared get", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: filesItemSuffix, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared patch", method: http.MethodPatch, visibility: database.FileVisibilityShared, suffix: filesItemSuffix, body: map[string]any{"name": "renamed.sql"}, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared delete", method: http.MethodDelete, visibility: database.FileVisibilityShared, suffix: filesItemSuffix, itemRoute: true, wantAllowedStatus: http.StatusNoContent, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared content get", method: http.MethodGet, visibility: database.FileVisibilityShared, suffix: filesContentSuffix, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
	{name: "shared content put", method: http.MethodPut, visibility: database.FileVisibilityShared, suffix: filesContentSuffix, contentRequest: true, itemRoute: true, wantAllowedStatus: http.StatusOK, wantMemberStatus: http.StatusForbidden, wantMemberCode: apiErrorNotPermitted},
}

func TestWorkspaceFileRoutesCharacterization(t *testing.T) {
	for _, route := range filesCharacterizationRoutes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			fixture := newFilesCharacterizationFixture(t)
			fileID := fixture.privateFileID
			if route.visibility == database.FileVisibilityShared {
				fileID = fixture.sharedFileID
			}
			baseURL := filesCharacterizationBaseURL(fixture, route.visibility)
			path := baseURL + route.suffix(fixture, fileID)
			noWSStatus, noWSCode := http.StatusForbidden, apiErrorNotPermitted
			if route.visibility == database.FileVisibilityPrivate && route.itemRoute {
				noWSStatus, noWSCode = http.StatusNotFound, apiErrorNotFound
			}
			cases := []struct {
				name       string
				token      string
				path       string
				wantStatus int
				wantCode   string
			}{
				{name: "unauthenticated", path: path, wantStatus: http.StatusUnauthorized, wantCode: apiErrorInvalidAuthenticationToken},
				{name: "non-member", token: fixture.nonMemberToken, path: path, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "org member without workspace access", token: fixture.memberToken, path: path, wantStatus: noWSStatus, wantCode: noWSCode},
			}
			if route.visibility == database.FileVisibilityShared {
				cases = append(cases, struct {
					name, token, path string
					wantStatus        int
					wantCode          string
				}{name: "workspace member without file permission", token: fixture.workspaceMemberToken, path: path, wantStatus: route.wantMemberStatus, wantCode: route.wantMemberCode})
			}
			if route.itemRoute {
				cases = append(cases,
					struct {
						name, token, path string
						wantStatus        int
						wantCode          string
					}{name: "file from another workspace", token: fixture.ownerToken, path: baseURL + route.suffix(fixture, fixture.foreignPrivateID), wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound},
				)
				if route.visibility == database.FileVisibilityPrivate {
					cases = append(cases,
						struct {
							name, token, path string
							wantStatus        int
							wantCode          string
						}{name: "another member private file", token: fixture.workspaceMemberToken, path: path, wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound},
					)
				}
			}
			cases = append(cases, struct {
				name, token, path string
				wantStatus        int
				wantCode          string
			}{name: "allowed", token: fixture.ownerToken, path: path, wantStatus: route.wantAllowedStatus})

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var req *http.Request
					if route.contentRequest {
						etag := fixture.privateETag
						if route.visibility == database.FileVisibilityShared {
							etag = fixture.sharedETag
						}
						req = newAuthContentRequest(t, route.method, tc.path, "select characterized", tc.token, etag)
					} else {
						req = newAuthRequest(t, route.method, tc.path, route.body, tc.token)
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

func filesCollectionSuffix(filesCharacterizationFixture, int64) string { return "" }

func filesItemSuffix(_ filesCharacterizationFixture, id int64) string {
	return "/" + strconv.FormatInt(id, 10)
}

func filesContentSuffix(_ filesCharacterizationFixture, id int64) string {
	return "/" + strconv.FormatInt(id, 10) + "/content"
}

func filesCharacterizationBaseURL(f filesCharacterizationFixture, visibility string) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/files/%s", f.org.Slug, f.workspace.ID, visibility)
}

func newFilesCharacterizationFixture(t *testing.T) filesCharacterizationFixture {
	t.Helper()
	app := newTestApp(t)
	owner, ownerToken, org := seedOrgOwner(t, app, uniqueEmail(t, "files-char-owner"), "Files Characterization Owner", "Files Characterization")
	member, memberToken := seedAccountWithToken(t, app, uniqueEmail(t, "files-char-member"), "Files Characterization Member")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	workspaceMember, workspaceMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "files-char-workspace-member"), "Files Characterization Workspace Member")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, workspaceMember.ID); err != nil {
		t.Fatal(err)
	}
	workspace := seedWorkspaceForAccount(t, app, org, owner, "Files Characterization Workspace", "")
	if err := testDB(app).AddWorkspaceMember(context.Background(), workspace.ID, workspaceMember.ID, nil); err != nil {
		t.Fatal(err)
	}
	_, nonMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "files-char-non-member"), "Files Characterization Non-member")

	privateFileID, privateETag := createCharacterizationFile(t, app, org, workspace, ownerToken, database.FileVisibilityPrivate, "private.sql")
	sharedFileID, sharedETag := createCharacterizationFile(t, app, org, workspace, ownerToken, database.FileVisibilityShared, "shared.sql")
	foreignWorkspace := seedWorkspaceForAccount(t, app, org, owner, "Files Characterization Foreign", "")
	foreignPrivateID, _ := createCharacterizationFile(t, app, org, foreignWorkspace, ownerToken, database.FileVisibilityPrivate, "foreign.sql")
	return filesCharacterizationFixture{
		app: app, ownerToken: ownerToken, memberToken: memberToken, workspaceMemberToken: workspaceMemberToken, nonMemberToken: nonMemberToken,
		org: org, workspace: workspace, privateFileID: privateFileID, sharedFileID: sharedFileID, foreignPrivateID: foreignPrivateID,
		privateETag: privateETag, sharedETag: sharedETag,
	}
}

func createCharacterizationFile(t *testing.T, app *application, org database.Organization, workspace database.Workspace, token, visibility, name string) (int64, string) {
	t.Helper()
	baseURL := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/files/%s", org.Slug, workspace.ID, visibility)
	res := send(t, newAuthRequest(t, http.MethodPost, baseURL, map[string]any{"name": name}, token), app.routes())
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create file status = %d", res.StatusCode)
	}
	id := int64(res.BodyFields["id"].(float64))
	write := send(t, newAuthContentRequest(t, http.MethodPut, baseURL+"/"+strconv.FormatInt(id, 10)+"/content", "select 1", token, ""), app.routes())
	if write.StatusCode != http.StatusOK {
		t.Fatalf("write file status = %d", write.StatusCode)
	}
	return id, write.Header.Get("ETag")
}
