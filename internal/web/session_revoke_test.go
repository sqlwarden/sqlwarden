package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/jobs"
)

type fakeRevoker struct {
	mu    sync.Mutex
	err   error
	count int
	calls []string
	ctxs  []context.Context
	// ctxErrs holds ctx.Err() as observed during each call.
	ctxErrs []error
}

func (f *fakeRevoker) record(ctx context.Context, call string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	f.ctxs = append(f.ctxs, ctx)
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	return 1, f.err
}

func (f *fakeRevoker) CountForConnection(ctx context.Context, connID string) (int, error) {
	f.record(ctx, "count:"+connID)
	return f.count, nil
}

func (f *fakeRevoker) RevokeConnection(ctx context.Context, connID string) (int, error) {
	return f.record(ctx, "connection:"+connID)
}

func (f *fakeRevoker) RevokeWorkspaceAccount(ctx context.Context, wsID, accountID string) (int, error) {
	return f.record(ctx, "workspace_account:"+wsID+":"+accountID)
}

func (f *fakeRevoker) RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error) {
	return f.record(ctx, "org_account:"+orgID+":"+accountID)
}

func sessionRevokeJobs(t *testing.T, app *application) []jobs.Record {
	t.Helper()
	var records []jobs.Record
	err := app.db.NewSelect().Model(&records).Where("type = ?", jobs.TypeSessionRevoke).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestSessionRevokeOrgMemberRemovalFailureEnqueuesJob(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	var logs bytes.Buffer
	app.logger = slog.New(slog.NewTextHandler(&logs, nil))
	fake := &fakeRevoker{err: errors.New("boom")}
	app.revoker = fake

	_, ownerTok, slug := registerAndLogin(t, app, "owner-revoke@example.com", "Owner", "securepass99")
	memberID, _, _ := registerAndLogin(t, app, "member-revoke@example.com", "Member", "securepass99")
	addOrgMemberDirect(t, app, slug, "member-revoke@example.com")

	req := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/members/"+memberID, nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}

	records := sessionRevokeJobs(t, app)
	if len(records) != 1 {
		t.Fatalf("session_revoke jobs = %d, want 1", len(records))
	}
	if records[0].Visibility != jobs.VisibilityInternal || records[0].Status != jobs.StatusQueued {
		t.Fatalf("unexpected job state %+v", records[0])
	}
	org, found, err := app.db.GetOrgBySlug(context.Background(), slug)
	if err != nil || !found {
		t.Fatalf("lookup org: found=%v err=%v", found, err)
	}
	orgID := strconv.FormatInt(org.ID, 10)
	wantInput := `{"kind":"org_account","org_id":"` + orgID + `","account_id":"` + memberID + `"}`
	if records[0].InputJSON != wantInput {
		t.Fatalf("input = %s, want %s", records[0].InputJSON, wantInput)
	}
	if records[0].SingletonKey != "session_revoke:org_account:"+orgID+":"+memberID {
		t.Fatalf("singleton key = %q", records[0].SingletonKey)
	}
	if !bytes.Contains(logs.Bytes(), []byte("session revocation failed")) {
		t.Fatalf("expected warning log, got %s", logs.String())
	}
}

func TestSessionRevokeOrgMemberRemovalSuccessEnqueuesNothing(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	fake := &fakeRevoker{}
	app.revoker = fake

	_, ownerTok, slug := registerAndLogin(t, app, "owner-revoke-ok@example.com", "Owner", "securepass99")
	memberID, _, _ := registerAndLogin(t, app, "member-revoke-ok@example.com", "Member", "securepass99")
	addOrgMemberDirect(t, app, slug, "member-revoke-ok@example.com")

	req := newTestRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/members/"+memberID, nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	if res := send(t, req, app.routes()); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("revoker calls = %v, want 1", fake.calls)
	}
	if got := sessionRevokeJobs(t, app); len(got) != 0 {
		t.Fatalf("session_revoke jobs = %d, want 0", len(got))
	}
}

func TestSessionRevokeSurvivesCancelledRequestContext(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	fake := &fakeRevoker{}
	app.revoker = fake

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodDelete, "/", nil).WithContext(ctx)

	app.revokeSessions(r, sessionRevokeInput{Kind: sessionRevokeOrgAccount, OrgID: "1", AccountID: "2"})

	if len(fake.ctxs) != 1 {
		t.Fatalf("revoker calls = %d, want 1", len(fake.ctxs))
	}
	if err := fake.ctxErrs[0]; err != nil {
		t.Fatalf("revoker context is done: %v", err)
	}
}

func TestSessionRevokeUsesRequestContextAndCoalescesFailures(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	fake := &fakeRevoker{err: errors.New("boom")}
	app.revoker = fake

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	r := httptest.NewRequest(http.MethodDelete, "/", nil).WithContext(ctx)
	in := sessionRevokeInput{Kind: sessionRevokeConnection, ConnectionID: "42"}

	app.revokeSessions(r, in)
	app.revokeSessions(r, in)

	for _, c := range fake.ctxs {
		if c.Value(ctxKey{}) != "marker" {
			t.Fatal("revoker did not receive the request context")
		}
	}
	records := sessionRevokeJobs(t, app)
	if len(records) != 1 {
		t.Fatalf("session_revoke jobs = %d, want 1", len(records))
	}
	if records[0].SingletonKey != "session_revoke:connection:42" {
		t.Fatalf("singleton key = %q", records[0].SingletonKey)
	}

	app.revokeSessions(r, sessionRevokeInput{Kind: sessionRevokeWorkspaceAccount, WorkspaceID: "7", AccountID: "9"})
	if got := sessionRevokeJobs(t, app); len(got) != 2 {
		t.Fatalf("session_revoke jobs = %d, want 2", len(got))
	}
}

func TestSessionRevokeJobHandler(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	fake := &fakeRevoker{err: errors.New("boom")}
	app.revoker = fake
	runtime := jobs.Runtime{Job: jobs.Record{InputJSON: `{"kind":"workspace_account","workspace_id":"7","account_id":"9"}`}}

	_, err := app.handleSessionRevoke(context.Background(), runtime)
	var coded jobs.CodedError
	if !errors.As(err, &coded) || coded.Code != "session_revoke_failed" || !coded.Retryable {
		t.Fatalf("err = %#v, want retryable session_revoke_failed", err)
	}
	if fake.calls[0] != "workspace_account:7:9" {
		t.Fatalf("calls = %v", fake.calls)
	}

	fake.err = nil
	out, err := app.handleSessionRevoke(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if out.(map[string]any)["revoked"] != 1 {
		t.Fatalf("output = %v", out)
	}

	_, err = app.handleSessionRevoke(context.Background(), jobs.Runtime{Job: jobs.Record{InputJSON: `{"kind":"nope"}`}})
	if !errors.As(err, &coded) || coded.Retryable {
		t.Fatalf("unknown kind err = %#v, want permanent", err)
	}
}

func TestSessionRevokeJobTypeIsRegistered(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	def, ok := app.defaultJobRegistry().Definition(jobs.TypeSessionRevoke)
	if !ok {
		t.Fatal("session_revoke job type is not registered")
	}
	if def.Handler == nil || def.MaxAttempts != sessionRevokeMaxAttempts || def.Backoff == nil || def.Backoff(1) <= 0 {
		t.Fatalf("incomplete definition: %+v", def)
	}
}

func TestSessionRevokeHandlersEnqueueRetryOnFailure(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*application, database.Organization, database.Workspace, string, database.Account, string) {
		app, org, ws, tok := setupWorkspaceOwner(t)
		app.revoker = &fakeRevoker{err: errors.New("boom"), count: 1}
		member, _ := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "revoke-member"))
		return app, org, ws, tok, member, strconv.FormatInt(member.ID, 10)
	}
	addTeam := func(t *testing.T, app *application, org database.Organization, ws database.Workspace, member database.Account) database.Team {
		team, err := app.db.InsertTeam(context.Background(), org.ID, "revoke-team", "Revoke Team")
		if err != nil {
			t.Fatal(err)
		}
		if err := app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
			t.Fatal(err)
		}
		if err := app.db.AddWorkspaceTeam(context.Background(), ws.ID, team.ID, nil); err != nil {
			t.Fatal(err)
		}
		return team
	}

	tests := []struct {
		name string
		run  func(t *testing.T, app *application, org database.Organization, ws database.Workspace, tok string, member database.Account) (*http.Response, []sessionRevokeInput)
	}{
		{
			name: "workspace member removal",
			run: func(t *testing.T, app *application, org database.Organization, ws database.Workspace, tok string, member database.Account) (*http.Response, []sessionRevokeInput) {
				res := send(t, newAuthRequest(t, http.MethodDelete,
					fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/users/%d", org.Slug, ws.ID, member.ID), nil, tok), app.routes())
				return res.Response, []sessionRevokeInput{{Kind: sessionRevokeWorkspaceAccount, WorkspaceID: strconv.FormatInt(ws.ID, 10), AccountID: strconv.FormatInt(member.ID, 10)}}
			},
		},
		{
			name: "workspace team removal",
			run: func(t *testing.T, app *application, org database.Organization, ws database.Workspace, tok string, member database.Account) (*http.Response, []sessionRevokeInput) {
				team := addTeam(t, app, org, ws, member)
				res := send(t, newAuthRequest(t, http.MethodDelete,
					fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/teams/%d", org.Slug, ws.ID, team.ID), nil, tok), app.routes())
				return res.Response, []sessionRevokeInput{{Kind: sessionRevokeWorkspaceAccount, WorkspaceID: strconv.FormatInt(ws.ID, 10), AccountID: strconv.FormatInt(member.ID, 10)}}
			},
		},
		{
			name: "team member removal",
			run: func(t *testing.T, app *application, org database.Organization, ws database.Workspace, tok string, member database.Account) (*http.Response, []sessionRevokeInput) {
				team := addTeam(t, app, org, ws, member)
				res := send(t, newAuthRequest(t, http.MethodDelete,
					fmt.Sprintf("/api/v1/orgs/%s/teams/%s/members/%d", org.Slug, team.Slug, member.ID), nil, tok), app.routes())
				return res.Response, []sessionRevokeInput{{Kind: sessionRevokeWorkspaceAccount, WorkspaceID: strconv.FormatInt(ws.ID, 10), AccountID: strconv.FormatInt(member.ID, 10)}}
			},
		},
		{
			name: "connection update with forced dsn rotation",
			run: func(t *testing.T, app *application, org database.Organization, ws database.Workspace, tok string, member database.Account) (*http.Response, []sessionRevokeInput) {
				envID := defaultEnvironmentID(t, app, ws.ID)
				createRes := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(org.Slug, ws.ID, envID),
					map[string]any{"name": "Primary", "driver": "sqlite", "dsn": ":memory:"}, tok), app.routes())
				if createRes.StatusCode != http.StatusCreated {
					t.Fatalf("create connection status = %d", createRes.StatusCode)
				}
				connID := fmt.Sprintf("%v", createRes.BodyFields["id"])
				res := send(t, newAuthRequest(t, http.MethodPatch, orgConnectionURL(org.Slug, ws.ID, envID, connID),
					map[string]any{"name": "Primary", "dsn": "file::memory:?cache=shared", "access_mode": "open", "force": true}, tok), app.routes())
				return res.Response, []sessionRevokeInput{{Kind: sessionRevokeConnection, ConnectionID: connID}}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app, org, ws, tok, member, _ := setup(t)
			res, want := tc.run(t, app, org, ws, tok, member)
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", res.StatusCode)
			}
			records := sessionRevokeJobs(t, app)
			if len(records) != len(want) {
				t.Fatalf("session_revoke jobs = %d, want %d", len(records), len(want))
			}
			wantJSON, err := json.Marshal(want[0])
			if err != nil {
				t.Fatal(err)
			}
			if records[0].InputJSON != string(wantJSON) {
				t.Fatalf("input = %s, want %s", records[0].InputJSON, wantJSON)
			}
			if records[0].SingletonKey != want[0].uniqueKey() {
				t.Fatalf("singleton key = %q, want %q", records[0].SingletonKey, want[0].uniqueKey())
			}
		})
	}
}
