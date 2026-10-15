package services

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

var errDirectoryStoreNotConfigured = errors.New("services: directory store is not configured")

// directoryStore is the metadata surface the access requirer resolves
// organizations, workspaces, and connections through.
type directoryStore interface {
	GetOrg(ctx context.Context, id int64) (database.Organization, bool, error)
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error)
	GetWorkspace(ctx context.Context, id int64) (database.Workspace, bool, error)
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
}

type directoryAdapter struct {
	store directoryStore
}

var _ access.Directory = directoryAdapter{}

func (d directoryAdapter) GetOrg(ctx context.Context, id int64) (access.Org, bool, error) {
	if d.store == nil {
		return access.Org{}, false, errDirectoryStoreNotConfigured
	}
	org, found, err := d.store.GetOrg(ctx, id)
	if err != nil || !found {
		return access.Org{}, found, err
	}
	return access.Org{ID: org.ID, Slug: org.Slug, Name: org.Name}, true, nil
}

func (d directoryAdapter) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	if d.store == nil {
		return false, errDirectoryStoreNotConfigured
	}
	return d.store.IsOrgMember(ctx, orgID, accountID)
}

func (d directoryAdapter) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	if d.store == nil {
		return false, errDirectoryStoreNotConfigured
	}
	return d.store.IsEffectiveWorkspaceMember(ctx, orgID, workspaceID, accountID)
}

func (d directoryAdapter) GetWorkspace(ctx context.Context, id int64) (access.Workspace, bool, error) {
	if d.store == nil {
		return access.Workspace{}, false, errDirectoryStoreNotConfigured
	}
	workspace, found, err := d.store.GetWorkspace(ctx, id)
	if err != nil || !found {
		return access.Workspace{}, found, err
	}
	return access.Workspace{
		ID:        workspace.ID,
		OrgID:     workspace.OrgID,
		OwnerType: workspace.OwnerType,
		OwnerID:   workspace.OwnerID,
		Name:      workspace.Name,
	}, true, nil
}

func (d directoryAdapter) GetConnection(ctx context.Context, id int64) (access.Connection, bool, error) {
	if d.store == nil {
		return access.Connection{}, false, errDirectoryStoreNotConfigured
	}
	connection, found, err := d.store.GetConnection(ctx, id)
	if err != nil || !found {
		return access.Connection{}, found, err
	}
	return access.Connection{
		ID:            connection.ID,
		WorkspaceID:   connection.WorkspaceID,
		EnvironmentID: connection.EnvironmentID,
		Name:          connection.Name,
		Driver:        connection.Driver,
	}, true, nil
}
