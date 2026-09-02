package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

func newDesktopTestApp(t *testing.T) *application {
	t.Helper()
	app := newTestApplication(t)
	if _, err := app.db.ExecContext(context.Background(), "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}
	enforcer, err := access.New(app.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	app.enforcer = enforcer
	app.config.Mode = ModeDesktop
	return app
}

func TestBootstrapDesktopCreatesStableAuthorizedIdentity(t *testing.T) {
	app := newDesktopTestApp(t)
	ctx := context.Background()

	first, err := app.BootstrapDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.BootstrapDesktop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("identity changed across bootstrap: first=%+v second=%+v", first, second)
	}
	if first.OrgSlug != singleUserDefaultOrgSlug || first.WorkspaceID != nil {
		t.Fatalf("unexpected desktop resources: %+v", first)
	}
	account, found, err := app.db.GetAccount(ctx, first.AccountID)
	if err != nil || !found {
		t.Fatalf("get local account: found=%v err=%v", found, err)
	}
	if account.Password != nil || account.Email != desktopAccountEmail {
		t.Fatalf("unexpected local account: %+v", account)
	}
	count, err := app.db.CountOrganizationWorkspaces(ctx, first.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("workspace count = %d, want 0", count)
	}
	if !app.enforcer.Can(ctx, first.AccountID, first.OrgID, "org", "org", first.OrgID, access.PermOrgRead) {
		t.Fatal("desktop account was not granted organization access")
	}
}

func TestBootstrapDesktopRefusesExistingUnownedData(t *testing.T) {
	app := newDesktopTestApp(t)
	if _, err := app.db.InsertAccount(context.Background(), "existing@example.com", "Existing", nil); err != nil {
		t.Fatal(err)
	}
	_, err := app.BootstrapDesktop(context.Background())
	if !errors.Is(err, ErrDesktopDatabaseNotPristine) {
		t.Fatalf("error = %v, want ErrDesktopDatabaseNotPristine", err)
	}
}

func TestDesktopSessionReusesValidNativeSession(t *testing.T) {
	app := newDesktopTestApp(t)
	ctx := context.Background()

	first, err := app.NewDesktopSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := app.RefreshDesktopSession(ctx, first.AuthSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AuthSessionID != first.AuthSessionID {
		t.Fatalf("session ID changed: got %q want %q", refreshed.AuthSessionID, first.AuthSessionID)
	}
	if refreshed.AccessToken == "" {
		t.Fatal("refreshed access token is empty")
	}
	if err := app.RevokeDesktopSession(ctx, first.AuthSessionID); err != nil {
		t.Fatal(err)
	}
	replacement, err := app.RefreshDesktopSession(ctx, first.AuthSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.AuthSessionID == first.AuthSessionID {
		t.Fatal("revoked native session was reused")
	}
}

func TestBootstrapDesktopRequiresDesktopMode(t *testing.T) {
	app := newDesktopTestApp(t)
	app.config.Mode = ModeServer
	_, err := app.BootstrapDesktop(context.Background())
	if !errors.Is(err, ErrDesktopModeRequired) {
		t.Fatalf("error = %v, want ErrDesktopModeRequired", err)
	}
}

func TestBootstrapDesktopRejectsTamperedIdentity(t *testing.T) {
	app := newDesktopTestApp(t)
	identity, err := app.BootstrapDesktop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.NewDelete().Model((*database.OrgMember)(nil)).
		Where("org_id = ? AND account_id = ?", identity.OrgID, identity.AccountID).
		Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = app.BootstrapDesktop(context.Background())
	if !errors.Is(err, ErrDesktopIdentityInvalid) {
		t.Fatalf("error = %v, want ErrDesktopIdentityInvalid", err)
	}
}

func TestDesktopCanDeleteLastWorkspace(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := app.createOwnedWorkspace(context.Background(), session.Identity.OrgID, session.Identity.AccountID, "Only workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d", session.Identity.OrgSlug, workspace.ID)
	response := send(t, newAuthRequest(t, http.MethodDelete, path, nil, session.AccessToken), app.routes())
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, http.StatusNoContent, response.BodyBytes)
	}
	count, err := app.db.CountOrganizationWorkspaces(context.Background(), session.Identity.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("workspace count = %d, want 0", count)
	}
}

func TestDesktopCanDeleteWorkspaceWhenAnotherRemains(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.createOwnedWorkspace(context.Background(), session.Identity.OrgID, session.Identity.AccountID, "First", ""); err != nil {
		t.Fatal(err)
	}
	second, err := app.createOwnedWorkspace(context.Background(), session.Identity.OrgID, session.Identity.AccountID, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/orgs/" + session.Identity.OrgSlug + "/workspaces/" + strconv.FormatInt(second.ID, 10)
	response := send(t, newAuthRequest(t, http.MethodDelete, path, nil, session.AccessToken), app.routes())
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, http.StatusNoContent, response.BodyBytes)
	}
	count, err := app.db.CountOrganizationWorkspaces(context.Background(), session.Identity.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("workspace count = %d, want 1", count)
	}
}

func TestDesktopServerOnlyRoutesAreNotRoutable(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/setup"},
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/orgs"},
		{http.MethodGet, "/api/v1/instance/admins"},
		{http.MethodPatch, "/api/v1/account/password"},
		{http.MethodGet, "/api/v1/me/workspaces"},
		{http.MethodGet, "/api/v1/orgs/" + session.Identity.OrgSlug + "/members"},
		{http.MethodGet, "/api/v1/orgs/" + session.Identity.OrgSlug + "/invitations"},
		{http.MethodGet, "/api/v1/orgs/" + session.Identity.OrgSlug + "/teams"},
		{http.MethodGet, "/api/v1/orgs/" + session.Identity.OrgSlug + "/roles"},
		{http.MethodGet, "/api/v1/orgs/" + session.Identity.OrgSlug + "/policies"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			response := send(t, newAuthRequest(t, tt.method, tt.path, nil, session.AccessToken), app.routes())
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, http.StatusNotFound, response.BodyBytes)
			}
		})
	}
}

func TestDesktopSharedSettingsAndWorkspaceRoutesRemainAvailable(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	paths := []string{
		"/api/v1/instance/settings",
		"/api/v1/orgs/" + session.Identity.OrgSlug,
		"/api/v1/orgs/" + session.Identity.OrgSlug + "/workspaces",
	}
	for _, path := range paths {
		response := send(t, newAuthRequest(t, http.MethodGet, path, nil, session.AccessToken), app.routes())
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d; body=%s", path, response.StatusCode, http.StatusOK, response.BodyBytes)
		}
	}
}
