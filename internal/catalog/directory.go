package catalog

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

var errStoreNotConfigured = errors.New("catalog: store is not configured")

// storeDirectory adapts the catalog Store to access.Directory. Org lookup and
// workspace-team membership are optional store capabilities because the
// connection paths only need org membership, workspaces and connections.
type storeDirectory struct {
	store Store
}

type orgReader interface {
	GetOrg(ctx context.Context, id int64) (database.Organization, bool, error)
}

type workspaceMemberReader interface {
	IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error)
}

func (d storeDirectory) GetOrg(ctx context.Context, id int64) (access.Org, bool, error) {
	reader, ok := d.store.(orgReader)
	if !ok {
		return access.Org{}, false, errors.New("catalog: store cannot read organizations")
	}
	org, found, err := reader.GetOrg(ctx, id)
	if err != nil || !found {
		return access.Org{}, found, err
	}
	return access.Org{ID: org.ID, Slug: org.Slug, Name: org.Name}, true, nil
}

func (d storeDirectory) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	if d.store == nil {
		return false, errStoreNotConfigured
	}
	return d.store.IsOrgMember(ctx, orgID, accountID)
}

func (d storeDirectory) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	reader, ok := d.store.(workspaceMemberReader)
	if !ok {
		return false, errors.New("catalog: store cannot read workspace membership")
	}
	return reader.IsEffectiveWorkspaceMember(ctx, orgID, workspaceID, accountID)
}

func (d storeDirectory) GetWorkspace(ctx context.Context, id int64) (access.Workspace, bool, error) {
	if d.store == nil {
		return access.Workspace{}, false, errStoreNotConfigured
	}
	ws, found, err := d.store.GetWorkspace(ctx, id)
	if err != nil || !found {
		return access.Workspace{}, found, err
	}
	return access.Workspace{ID: ws.ID, OrgID: ws.OrgID, OwnerType: ws.OwnerType, OwnerID: ws.OwnerID, Name: ws.Name}, true, nil
}

func (d storeDirectory) GetConnection(ctx context.Context, id int64) (access.Connection, bool, error) {
	if d.store == nil {
		return access.Connection{}, false, errStoreNotConfigured
	}
	conn, found, err := d.store.GetConnection(ctx, id)
	if err != nil || !found {
		return access.Connection{}, found, err
	}
	return access.Connection{ID: conn.ID, WorkspaceID: conn.WorkspaceID, EnvironmentID: conn.EnvironmentID, Name: conn.Name, Driver: conn.Driver}, true, nil
}
