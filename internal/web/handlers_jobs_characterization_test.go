package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
)

type jobsCharacterizationFixture struct {
	app            *application
	ownerToken     string
	memberToken    string
	nonMemberToken string
	org            database.Organization
	workspace      database.Workspace
	ownerJobID     string
	otherJobID     string
	foreignJobID   string
}

func TestJobRoutesCharacterization(t *testing.T) {
	routes := []struct {
		name   string
		method string
		suffix func(jobsCharacterizationFixture, string) string
		item   bool
	}{
		{name: "list jobs", method: http.MethodGet, suffix: func(jobsCharacterizationFixture, string) string { return "" }},
		{name: "get job", method: http.MethodGet, suffix: func(_ jobsCharacterizationFixture, id string) string { return "/" + id }, item: true},
		{name: "list job events", method: http.MethodGet, suffix: func(_ jobsCharacterizationFixture, id string) string { return "/" + id + "/events" }, item: true},
		{name: "cancel job", method: http.MethodPost, suffix: func(_ jobsCharacterizationFixture, id string) string { return "/" + id + "/cancel" }, item: true},
	}
	for _, route := range routes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			fixture := newJobsCharacterizationFixture(t)
			baseURL := "/api/v1/orgs/" + fixture.org.Slug + "/workspaces/" + strconv.FormatInt(fixture.workspace.ID, 10) + "/jobs"
			path := baseURL + route.suffix(fixture, fixture.ownerJobID)
			cases := []struct {
				name       string
				token      string
				path       string
				wantStatus int
				wantCode   string
			}{
				{name: "unauthenticated", path: path, wantStatus: http.StatusUnauthorized, wantCode: apiErrorInvalidAuthenticationToken},
				{name: "non-member", token: fixture.nonMemberToken, path: path, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
				{name: "org member without workspace access", token: fixture.memberToken, path: path, wantStatus: http.StatusForbidden, wantCode: apiErrorNotPermitted},
			}
			if route.item {
				cases = append(cases, struct {
					name, token, path string
					wantStatus        int
					wantCode          string
				}{
					name: "another account job", token: fixture.ownerToken, path: baseURL + route.suffix(fixture, fixture.otherJobID), wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound,
				})
				cases = append(cases, struct {
					name, token, path string
					wantStatus        int
					wantCode          string
				}{
					name: "foreign workspace job", token: fixture.ownerToken, path: baseURL + route.suffix(fixture, fixture.foreignJobID), wantStatus: http.StatusNotFound, wantCode: apiErrorNotFound,
				})
			}
			cases = append(cases, struct {
				name, token, path string
				wantStatus        int
				wantCode          string
			}{
				name: "allowed", token: fixture.ownerToken, path: path, wantStatus: http.StatusOK,
			})
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					res := send(t, newAuthRequest(t, route.method, tc.path, nil, tc.token), fixture.app.routes())
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

// The service authorizes before it validates paging, so a caller without
// workspace access sees 403 whether or not the query is valid, and a caller
// with access sees 422 for a bad one.
func TestJobListQueryAuthorizationPrecedesValidation(t *testing.T) {
	fixture := newJobsCharacterizationFixture(t)
	baseURL := "/api/v1/orgs/" + fixture.org.Slug + "/workspaces/" + strconv.FormatInt(fixture.workspace.ID, 10) + "/jobs"
	paths := map[string]string{
		"list jobs":       baseURL,
		"list job events": baseURL + "/" + fixture.ownerJobID + "/events",
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			denied := send(t, newAuthRequest(t, http.MethodGet, path+"?page_size=0", nil, fixture.memberToken), fixture.app.routes())
			if denied.StatusCode != http.StatusForbidden {
				t.Fatalf("invalid query without access: status = %d, want 403", denied.StatusCode)
			}
			invalid := send(t, newAuthRequest(t, http.MethodGet, path+"?page_size=0", nil, fixture.ownerToken), fixture.app.routes())
			if invalid.StatusCode != http.StatusUnprocessableEntity || invalid.ErrorCode() != "validation_failed" {
				t.Fatalf("invalid query: status = %d, code = %q, want 422 validation_failed", invalid.StatusCode, invalid.ErrorCode())
			}
			valid := send(t, newAuthRequest(t, http.MethodGet, path, nil, fixture.memberToken), fixture.app.routes())
			if valid.StatusCode != http.StatusForbidden {
				t.Fatalf("valid query: status = %d, want 403", valid.StatusCode)
			}
		})
	}
}

func newJobsCharacterizationFixture(t *testing.T) jobsCharacterizationFixture {
	t.Helper()
	app := newTestApp(t)
	owner, ownerToken, org := seedOrgOwner(t, app, uniqueEmail(t, "jobs-char-owner"), "Jobs Characterization Owner", "Jobs Characterization")
	member, memberToken := seedAccountWithToken(t, app, uniqueEmail(t, "jobs-char-member"), "Jobs Characterization Member")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	other, _ := seedAccountWithToken(t, app, uniqueEmail(t, "jobs-char-other"), "Jobs Characterization Other")
	if err := testDB(app).AddOrgMember(context.Background(), org.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	_, nonMemberToken := seedAccountWithToken(t, app, uniqueEmail(t, "jobs-char-non-member"), "Jobs Characterization Non-member")
	workspace := seedWorkspaceForAccount(t, app, org, owner, "Jobs Characterization Workspace", "")
	foreignWorkspace := seedWorkspaceForAccount(t, app, org, owner, "Foreign Jobs Characterization Workspace", "")
	if err := testDB(app).AddWorkspaceMember(context.Background(), workspace.ID, other.ID, nil); err != nil {
		t.Fatal(err)
	}

	store := jobs.NewStore(testDB(app))
	ownerJob, err := store.Enqueue(context.Background(), jobs.EnqueueInput{
		Type: "noop", Visibility: jobs.VisibilityUser, OrgID: &org.ID, WorkspaceID: &workspace.ID,
		OwnerAccountID: &owner.ID, RunAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(context.Background(), jobs.EventInput{JobID: ownerJob.ID, Level: jobs.EventLevelInfo, Code: "characterized", Message: "Characterized."}); err != nil {
		t.Fatal(err)
	}
	otherJob, err := store.Enqueue(context.Background(), jobs.EnqueueInput{
		Type: "noop", Visibility: jobs.VisibilityUser, OrgID: &org.ID, WorkspaceID: &workspace.ID,
		OwnerAccountID: &other.ID, RunAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignJob, err := store.Enqueue(context.Background(), jobs.EnqueueInput{
		Type: "noop", Visibility: jobs.VisibilityUser, OrgID: &org.ID, WorkspaceID: &foreignWorkspace.ID,
		OwnerAccountID: &owner.ID, RunAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return jobsCharacterizationFixture{
		app: app, ownerToken: ownerToken, memberToken: memberToken, nonMemberToken: nonMemberToken,
		org: org, workspace: workspace, ownerJobID: ownerJob.ID, otherJobID: otherJob.ID, foreignJobID: foreignJob.ID,
	}
}
