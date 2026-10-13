package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/settings"
)

func firstRunInput(email, slug string) identity.FirstRun {
	return identity.FirstRun{
		Email: email, Name: "Admin", OrgSlug: slug, OrgName: "First Organization",
		SessionExpiresAt: time.Now().Add(time.Hour), UserAgent: "agent", IPAddress: "127.0.0.1",
		Auth: database.SessionAuth{Method: identity.MethodPassword, Assurance: string(access.AAL1)},
	}
}

func TestFirstRunStoreRollsBackAccountAdminAndSessionOnOrgFailure(t *testing.T) {
	deps := newTestDeps(t)
	store := firstRunStore{db: deps.DB, enforcer: deps.Enforcer}
	ctx := context.Background()
	if _, err := deps.DB.InsertOrg(ctx, "first-org", "Existing Organization"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateFirstRun(ctx, firstRunInput("rollback-admin@example.com", "first-org")); err == nil {
		t.Fatal("expected duplicate organization failure")
	}

	if _, found, err := deps.DB.GetAccountByEmail(ctx, "rollback-admin@example.com"); err != nil || found {
		t.Fatalf("account found=%v err=%v", found, err)
	}
	admins, err := deps.DB.CountInstanceAdmins(ctx)
	if err != nil || admins != 0 {
		t.Fatalf("admins=%d err=%v", admins, err)
	}
	var sessions int
	if err := deps.DB.NewSelect().TableExpr("auth_sessions").ColumnExpr("COUNT(*)").Scan(ctx, &sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
}

func TestFirstRunStoreRefusesConfiguredInstance(t *testing.T) {
	deps := newTestDeps(t)
	store := firstRunStore{db: deps.DB, enforcer: deps.Enforcer}
	ctx := context.Background()

	records, err := store.CreateFirstRun(ctx, firstRunInput("first@example.com", "first-org"))
	if err != nil {
		t.Fatal(err)
	}
	if records.Account.ID == 0 || records.Org.Slug != "first-org" || records.AuthSession.ID == "" {
		t.Fatalf("records = %+v", records)
	}
	isAdmin, err := deps.DB.IsInstanceAdmin(ctx, records.Account.ID)
	if err != nil || !isAdmin {
		t.Fatalf("admin=%v err=%v", isAdmin, err)
	}

	if _, err := store.CreateFirstRun(ctx, firstRunInput("second@example.com", "second-org")); !errors.Is(err, identity.ErrAlreadyConfigured) {
		t.Fatalf("second run err = %v", err)
	}
	if _, found, _ := deps.DB.GetAccountByEmail(ctx, "second@example.com"); found {
		t.Fatal("second run created an account")
	}
}

func TestIdentitySetupSeedsOwnedOrganization(t *testing.T) {
	deps := newTestDeps(t)
	ctx := context.Background()
	if err := settings.InitializeInstanceBaseURL(ctx, deps.DB, "http://localhost:4444"); err != nil {
		t.Fatal(err)
	}
	deps.Config.JWT.SecretKey = "services-setup-test-secret-key-32"
	svc, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.Identity.Setup(ctx, identity.SetupInput{
		Name: "Admin", Email: "admin@example.com", Password: "admin-password", OrganizationName: "Acme",
	}, access.RequestAttributes{})
	if err != nil {
		t.Fatal(err)
	}
	owner := accountPrincipal(result.Account.ID)
	if _, err := svc.Requirer.Org(ctx, owner, result.Org.ID); err != nil {
		t.Fatalf("owner is not an org member: %v", err)
	}
	if !svc.Requirer.Can(ctx, owner, result.Org.ID, "org", "org", result.Org.ID, access.PermOrgDelete) {
		t.Fatal("owner policy was not seeded")
	}
}
