package catalog_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
)

func TestCreationPreservesHierarchyAndPolicyInvariants(t *testing.T) {
	fixture := newCatalogFixture(t)
	ctx := context.Background()

	owner, err := fixture.db.InsertAccount(ctx, "catalog-owner@example.com", "Catalog Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := fixture.service.CreateOrganization(ctx, catalog.CreateOrganizationInput{
		Name: "Catalog Org", Slug: "catalog-org", OwnerAccountID: owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := catalog.Actor{AccountID: owner.ID, OrgID: org.ID}
	workspace, err := fixture.service.CreateWorkspace(ctx, actor, org.ID, catalog.CreateWorkspaceInput{Name: "Primary"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := fixture.service.CreateEnvironment(ctx, actor, workspace.ID, catalog.CreateEnvironmentInput{Name: "Production"})
	if err != nil {
		t.Fatal(err)
	}
	connectionRow, err := fixture.service.CreateConnection(ctx, actor, workspace.ID, catalog.CreateConnectionInput{
		Name: "Primary DB", Driver: "postgres", DSN: "postgres://catalog.invalid/app", EnvironmentID: &environment.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, resource := range []struct {
		kind string
		id   int64
	}{{"workspace", workspace.ID}, {"environment", environment.ID}, {"connection", connectionRow.ID}} {
		if got := countRows(t, fixture.db, "resource_hierarchy", "child_type = ? AND child_id = ?", resource.kind, resource.id); got != 1 {
			t.Fatalf("%s %d hierarchy rows = %d, want 1", resource.kind, resource.id, got)
		}
	}
	if got := countRows(t, fixture.db, "roles", "org_id = ? AND workspace_id = ? AND is_builtin = ?", org.ID, workspace.ID, true); got == 0 {
		t.Fatal("workspace creation did not seed builtin roles")
	}
	if got := countRows(t, fixture.db, "role_bindings", "org_id = ? AND resource_type = ? AND resource_id = ?", org.ID, "workspace", workspace.ID); got == 0 {
		t.Fatal("workspace creation did not seed builtin policy bindings")
	}
	if connectionRow.DSNEncrypted == "postgres://catalog.invalid/app" {
		t.Fatal("connection DSN reached persistence without sealing")
	}
}

func TestCrossWorkspaceResourcesAreIndistinguishableFromMissing(t *testing.T) {
	fixture := newCatalogFixture(t)
	ctx := context.Background()
	owner, err := fixture.db.InsertAccount(ctx, "isolation-owner@example.com", "Isolation Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := fixture.service.CreateOrganization(ctx, catalog.CreateOrganizationInput{
		Name: "Isolation Org", Slug: "isolation-org", OwnerAccountID: owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := catalog.Actor{AccountID: owner.ID, OrgID: org.ID}
	first, err := fixture.service.CreateWorkspace(ctx, actor, org.ID, catalog.CreateWorkspaceInput{Name: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.service.CreateWorkspace(ctx, actor, org.ID, catalog.CreateWorkspaceInput{Name: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	foreignEnvironment, err := fixture.service.CreateEnvironment(ctx, actor, second.ID, catalog.CreateEnvironmentInput{Name: "Foreign"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = fixture.service.CreateConnection(ctx, actor, first.ID, catalog.CreateConnectionInput{
		Name: "Wrong Parent", Driver: "postgres", DSN: "postgres://catalog.invalid/app", EnvironmentID: &foreignEnvironment.ID,
	})
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("cross-workspace connection error = %v, want %v", err, catalog.ErrNotFound)
	}
	if got := countRows(t, fixture.db, "connections", "name = ?", "Wrong Parent"); got != 0 {
		t.Fatalf("cross-workspace connection wrote %d rows", got)
	}

	_, err = fixture.service.Environment(ctx, owner.ID, org.ID, first, foreignEnvironment)
	if !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("cross-workspace environment read error = %v, want %v", err, catalog.ErrNotFound)
	}
}

func TestDeleteInvalidatesResourceAncestry(t *testing.T) {
	fixture := newCatalogFixture(t)
	ctx := context.Background()
	owner, err := fixture.db.InsertAccount(ctx, "delete-owner@example.com", "Delete Owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := fixture.service.CreateOrganization(ctx, catalog.CreateOrganizationInput{
		Name: "Delete Org", Slug: "delete-org", OwnerAccountID: owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := catalog.Actor{AccountID: owner.ID, OrgID: org.ID}
	workspace, err := fixture.service.CreateWorkspace(ctx, actor, org.ID, catalog.CreateWorkspaceInput{Name: "Delete Workspace"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := fixture.service.CreateEnvironment(ctx, actor, workspace.ID, catalog.CreateEnvironmentInput{Name: "Disposable"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.DeleteEnvironment(ctx, actor, environment); err != nil {
		t.Fatal(err)
	}
	if !fixture.grants.invalidated("environment", environment.ID) {
		t.Fatal("environment delete did not invalidate ancestry")
	}

	connectionRow, err := fixture.service.CreateConnection(ctx, actor, workspace.ID, catalog.CreateConnectionInput{
		Name: "Disposable DB", Driver: "postgres", DSN: "postgres://catalog.invalid/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.DeleteConnection(ctx, actor, connectionRow); err != nil {
		t.Fatal(err)
	}
	if !fixture.grants.invalidated("connection", connectionRow.ID) {
		t.Fatal("connection delete did not invalidate ancestry")
	}
}

type catalogFixture struct {
	db      *database.DB
	service *catalog.Service
	grants  *recordingGrants
}

func newCatalogFixture(t *testing.T) catalogFixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "catalog.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	enforcer, err := access.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	sessions := connection.New(time.Minute)
	t.Cleanup(func() { sessions.Close() })
	grants := &recordingGrants{}
	service := catalog.NewService(
		catalog.NewDatabaseStore(db, enforcer), grants, sessions, keyring,
		catalog.NewTargetPolicy(nil), audit.Discard,
	)
	return catalogFixture{db: db, service: service, grants: grants}
}

type ancestryInvalidation struct {
	resourceType string
	resourceID   int64
}

type recordingGrants struct {
	ancestry []ancestryInvalidation
}

func (*recordingGrants) InvalidateOrgPolicy(int64)         {}
func (*recordingGrants) InvalidatePrincipals(int64, int64) {}
func (g *recordingGrants) InvalidateAncestry(kind string, id int64) {
	g.ancestry = append(g.ancestry, ancestryInvalidation{kind, id})
}

func (g *recordingGrants) invalidated(kind string, id int64) bool {
	for _, invalidation := range g.ancestry {
		if invalidation.resourceType == kind && invalidation.resourceID == id {
			return true
		}
	}
	return false
}

func countRows(t *testing.T, db *database.DB, table, where string, args ...any) int {
	t.Helper()
	count, err := db.NewSelect().TableExpr(table).Where(where, args...).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return count
}
