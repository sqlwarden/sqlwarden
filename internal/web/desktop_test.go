package web

import (
	"context"
	"errors"
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
	app.config.DeploymentMode = DeploymentModeDesktop
	app.config.AccessMode = AccessModeSingleUser
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
	if first.OrgSlug != singleUserDefaultOrgSlug || first.WorkspaceID == 0 {
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
	if count != 1 {
		t.Fatalf("workspace count = %d, want 1", count)
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

func TestBootstrapDesktopRequiresDesktopSingleUserTopology(t *testing.T) {
	app := newDesktopTestApp(t)
	app.config.DeploymentMode = DeploymentModeServer
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

func TestDesktopCannotDeleteLastWorkspace(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/orgs/" + session.Identity.OrgSlug + "/workspaces/" + strconv.FormatInt(session.Identity.WorkspaceID, 10)
	response := send(t, newAuthRequest(t, http.MethodDelete, path, nil, session.AccessToken), app.routes())
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, http.StatusConflict, response.BodyBytes)
	}
	count, err := app.db.CountOrganizationWorkspaces(context.Background(), session.Identity.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("workspace count = %d, want 1", count)
	}
}

func TestDesktopCanDeleteWorkspaceWhenAnotherRemains(t *testing.T) {
	app := newDesktopTestApp(t)
	session, err := app.NewDesktopSession(context.Background())
	if err != nil {
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
