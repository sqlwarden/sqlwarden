package services

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
)

type workspaceRevoker struct {
	execution.Revoker
	mu    sync.Mutex
	err   error
	calls [][2]string
}

func (r *workspaceRevoker) RevokeWorkspaceAccount(_ context.Context, workspaceID, accountID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{workspaceID, accountID})
	return 1, r.err
}

func hierarchyRows(t *testing.T, deps Deps, childType string, childID int64, parentType string, parentID int64) int {
	t.Helper()
	n, err := deps.DB.NewSelect().TableExpr("resource_hierarchy").
		Where("child_type = ? AND child_id = ? AND parent_type = ? AND parent_id = ?", childType, childID, parentType, parentID).
		Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func workspaceRoleCount(t *testing.T, deps Deps, orgID int64, workspaceID *int64) int {
	t.Helper()
	q := deps.DB.NewSelect().TableExpr("roles").Where("org_id = ?", orgID)
	if workspaceID == nil {
		q = q.Where("workspace_id IS NOT NULL")
	} else {
		q = q.Where("workspace_id = ?", *workspaceID)
	}
	n, err := q.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCatalogCreateWorkspaceWritesHierarchyAndSeedsRoles(t *testing.T) {
	deps := newTestDeps(t)
	svc, infra, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "create-workspace")

	ws, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ws.EnvironmentCount, 1)
	assert.Equal(t, hierarchyRows(t, deps, "workspace", ws.ID, "org", org.ID), 1)
	if workspaceRoleCount(t, deps, org.ID, &ws.ID) == 0 {
		t.Fatal("workspace builtin roles were not seeded")
	}
	if !infra.Policy.Can(ctx, owner.ID, org.ID, "org", "workspace", ws.ID, "ws:delete") {
		t.Fatal("creator was not bound as workspace administrator")
	}

	env, err := svc.Catalog.CreateEnvironment(ctx, accountPrincipal(owner.ID), org.ID, ws.ID, catalog.EnvironmentInput{Name: "Staging"})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hierarchyRows(t, deps, "environment", env.ID, "workspace", ws.ID), 1)

	if err := svc.Catalog.DeleteWorkspace(ctx, accountPrincipal(owner.ID), org.ID, ws.ID); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, hierarchyRows(t, deps, "workspace", ws.ID, "org", org.ID), 0)
}

func TestWorkspaceStoreCreateOwnedWorkspaceRollsBackOnSeedFailure(t *testing.T) {
	deps := newTestDeps(t)
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, _ := seedOwnedOrg(t, svc, deps, "workspace-rollback")

	store := workspaceStore{DB: deps.DB, enforcer: deps.Enforcer}
	if _, err := store.CreateOwnedWorkspace(ctx, org.ID, 99999999, "Rollback Workspace", ""); err == nil {
		t.Fatal("expected workspace seed failure")
	}

	workspaces, err := deps.DB.ListWorkspacesPage(ctx, database.ListWorkspacesParams{OwnerType: "org", OwnerID: org.ID, Name: "Rollback Workspace", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, workspaces.Total, 0)
	assert.Equal(t, workspaceRoleCount(t, deps, org.ID, nil), 0)
}

func TestCatalogListWorkspacesExcludesUnreadableWorkspaces(t *testing.T) {
	deps := newTestDeps(t)
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "list-workspaces")
	member, err := deps.DB.InsertAccount(ctx, "list-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	visible, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Visible"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Hidden"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Catalog.AddWorkspaceMember(ctx, accountPrincipal(owner.ID), org.ID, visible.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	page, err := svc.Catalog.ListWorkspaces(ctx, accountPrincipal(member.ID), org.ID, catalog.ListQuery{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, page.Total, 1)
	assert.Equal(t, page.Items[0].ID, visible.ID)

	ownerPage, err := svc.Catalog.ListWorkspaces(ctx, accountPrincipal(owner.ID), org.ID, catalog.ListQuery{Page: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ownerPage.Total, 2)
}

func TestCatalogWorkspaceMembershipRequiresOrgMemberAndRevokesOnRemoval(t *testing.T) {
	deps := newTestDeps(t)
	revoker := &workspaceRevoker{Revoker: deps.Runtime}
	deps.Revoker = revoker
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "workspace-members")
	ws, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := deps.DB.InsertAccount(ctx, "ws-outsider@example.com", "Outsider", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Catalog.AddWorkspaceMember(ctx, accountPrincipal(owner.ID), org.ID, ws.ID, outsider.ID)
	assert.ErrorIs(t, err, catalog.ErrNotFound)

	member, err := deps.DB.InsertAccount(ctx, "ws-member@example.com", "Member", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.DB.AddOrgMember(ctx, org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, svc.Catalog.AddWorkspaceMember(ctx, accountPrincipal(owner.ID), org.ID, ws.ID, member.ID))
	assert.Nil(t, svc.Catalog.RemoveWorkspaceMember(ctx, accountPrincipal(owner.ID), org.ID, ws.ID, member.ID))

	want := [2]string{strconv.FormatInt(ws.ID, 10), strconv.FormatInt(member.ID, 10)}
	if len(revoker.calls) != 1 || revoker.calls[0] != want {
		t.Fatalf("revoke calls = %v, want [%v]", revoker.calls, want)
	}
	assert.Equal(t, sessionRevokeJobs(t, deps), 0)
}

func TestFilesForPrincipalDeniesOutsidersAndForeignWorkspaces(t *testing.T) {
	deps := newTestDeps(t)
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "files-principal")
	other, _ := seedOwnedOrg(t, svc, deps, "files-principal-other")
	foreign, err := deps.DB.InsertWorkspace(ctx, &other.ID, "org", other.ID, "Foreign", "")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := deps.DB.InsertAccount(ctx, "files-outsider@example.com", "Outsider", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.FilesForPrincipal(ctx, accountPrincipal(outsider.ID), org.ID, ws.ID)
	assert.ErrorIs(t, err, access.ErrForbidden)

	_, err = svc.FilesForPrincipal(ctx, accountPrincipal(owner.ID), org.ID, foreign.ID)
	assert.ErrorIs(t, err, access.ErrNotFound)
}

func TestCatalogWorkspaceMemberRemovalQueuesRetryWhenRevocationFails(t *testing.T) {
	deps := newTestDeps(t)
	deps.Revoker = &workspaceRevoker{Revoker: deps.Runtime, err: errors.New("runtime unavailable")}
	svc, _, err := Build(deps)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	org, owner := seedOwnedOrg(t, svc, deps, "workspace-revoke-retry")
	ws, err := svc.Catalog.CreateWorkspace(ctx, accountPrincipal(owner.ID), org.ID, catalog.CreateWorkspaceInput{Name: "Engineering"})
	if err != nil {
		t.Fatal(err)
	}

	assert.Nil(t, svc.Catalog.RemoveWorkspaceMember(ctx, accountPrincipal(owner.ID), org.ID, ws.ID, owner.ID))
	assert.Equal(t, sessionRevokeJobs(t, deps), 1)
}
