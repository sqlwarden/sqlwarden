package access

import (
	"context"
	"slices"
)

// Org, Workspace and Connection are the resource facts the Requirer needs.
// They are access-owned because internal/database imports this package, so
// the Requirer cannot name database row types; callers adapt their store to
// Directory instead.
type Org struct {
	ID   int64
	Slug string
	Name string
}

type Workspace struct {
	ID        int64
	OrgID     *int64
	OwnerType string
	OwnerID   int64
	Name      string
}

type Connection struct {
	ID            int64
	WorkspaceID   int64
	EnvironmentID int64
	Name          string
	Driver        string
}

// Directory resolves the resources a Requirer authorizes against.
type Directory interface {
	GetOrg(ctx context.Context, id int64) (Org, bool, error)
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error)
	GetWorkspace(ctx context.Context, id int64) (Workspace, bool, error)
	GetConnection(ctx context.Context, id int64) (Connection, bool, error)
}

// ConnRef identifies a connection and the routed hierarchy used to reach it.
// EnvironmentID is optional for workspace-level connection routes.
type ConnRef struct {
	OrgID         int64
	WorkspaceID   int64
	EnvironmentID *int64
	ConnectionID  int64
}

// Requirer resolves a resource from a routed reference and authorizes the
// principal against it. Org membership is always the first gate, and a
// resource outside the routed hierarchy is reported as ErrNotFound so its
// existence is not disclosed through a different error.
type Requirer struct {
	dir    Directory
	policy PolicyEvaluator
}

func NewRequirer(dir Directory, policy PolicyEvaluator) *Requirer {
	return &Requirer{dir: dir, policy: policy}
}

func (r *Requirer) member(ctx context.Context, p Principal, orgID int64) error {
	if p.Subject.Kind != SubjectAccount {
		return ErrForbidden
	}
	ok, err := r.dir.IsOrgMember(ctx, orgID, p.Subject.ID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (r *Requirer) can(ctx context.Context, p Principal, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return r.policy != nil && r.policy.Can(ctx, p.Subject.ID, orgID, ownerType, resourceType, resourceID, permission)
}

// Org returns the organization when the principal is a member of it.
func (r *Requirer) Org(ctx context.Context, p Principal, orgID int64) (Org, error) {
	if err := r.member(ctx, p, orgID); err != nil {
		return Org{}, err
	}
	org, found, err := r.dir.GetOrg(ctx, orgID)
	if err != nil {
		return Org{}, err
	}
	if !found {
		return Org{}, ErrNotFound
	}
	return org, nil
}

func (r *Requirer) orgWorkspace(ctx context.Context, p Principal, orgID, workspaceID int64) (Workspace, error) {
	if err := r.member(ctx, p, orgID); err != nil {
		return Workspace{}, err
	}
	ws, found, err := r.dir.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if !found || ws.OwnerType != "org" || ws.OrgID == nil || *ws.OrgID != orgID {
		return Workspace{}, ErrNotFound
	}
	return ws, nil
}

// Workspace returns an org-owned workspace the principal may access. An empty
// permission requires only org membership; otherwise the principal must hold
// the permission on the workspace.
func (r *Requirer) Workspace(ctx context.Context, p Principal, orgID, workspaceID int64, permission string) (Workspace, error) {
	ws, err := r.orgWorkspace(ctx, p, orgID, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if permission != "" && !r.can(ctx, p, orgID, ws.OwnerType, "workspace", ws.ID, permission) {
		return Workspace{}, ErrForbidden
	}
	return ws, nil
}

// WorkspaceMember returns an org-owned workspace the principal belongs to,
// directly or through a workspace team.
func (r *Requirer) WorkspaceMember(ctx context.Context, p Principal, orgID, workspaceID int64) (Workspace, error) {
	ws, err := r.orgWorkspace(ctx, p, orgID, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	ok, err := r.dir.IsEffectiveWorkspaceMember(ctx, orgID, ws.ID, p.Subject.ID)
	if err != nil {
		return Workspace{}, err
	}
	if !ok {
		return Workspace{}, ErrForbidden
	}
	return ws, nil
}

func (r *Requirer) resolveConnection(ctx context.Context, p Principal, ref ConnRef) (Connection, Workspace, error) {
	ws, err := r.orgWorkspace(ctx, p, ref.OrgID, ref.WorkspaceID)
	if err != nil {
		return Connection{}, Workspace{}, err
	}
	conn, found, err := r.dir.GetConnection(ctx, ref.ConnectionID)
	if err != nil {
		return Connection{}, Workspace{}, err
	}
	if !found || conn.WorkspaceID != ref.WorkspaceID || (ref.EnvironmentID != nil && conn.EnvironmentID != *ref.EnvironmentID) {
		return Connection{}, Workspace{}, ErrNotFound
	}
	return conn, ws, nil
}

// ResolveConnection returns the connection and its workspace when the
// principal is an org member and the connection sits in the routed hierarchy.
// It checks no permission, for operations whose authorization is owned by the
// execution runtime, such as continuing a cursor bound to the session that
// opened it or closing one's own session.
func (r *Requirer) ResolveConnection(ctx context.Context, p Principal, ref ConnRef) (Connection, Workspace, error) {
	return r.resolveConnection(ctx, p, ref)
}

// Can reports whether the principal holds permission on a resource. It does
// not resolve the resource or check org membership, so callers must already
// have obtained the resource through another Requirer method.
func (r *Requirer) Can(ctx context.Context, p Principal, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return r.can(ctx, p, orgID, ownerType, resourceType, resourceID, permission)
}

// ConnectionMember returns the connection and its workspace when the principal
// is a member of the workspace, directly or through a workspace team. It
// requires no connection permission, for per-user data attached to a
// connection such as query history.
func (r *Requirer) ConnectionMember(ctx context.Context, p Principal, ref ConnRef) (Connection, Workspace, error) {
	conn, ws, err := r.resolveConnection(ctx, p, ref)
	if err != nil {
		return Connection{}, Workspace{}, err
	}
	ok, err := r.dir.IsEffectiveWorkspaceMember(ctx, ref.OrgID, ws.ID, p.Subject.ID)
	if err != nil {
		return Connection{}, Workspace{}, err
	}
	if !ok {
		return Connection{}, Workspace{}, ErrForbidden
	}
	return conn, ws, nil
}

// Connection returns the connection and its workspace when the principal holds
// at least one of anyOf on the connection. An empty anyOf grants nothing.
func (r *Requirer) Connection(ctx context.Context, p Principal, ref ConnRef, anyOf ...string) (Connection, Workspace, error) {
	conn, ws, err := r.resolveConnection(ctx, p, ref)
	if err != nil {
		return Connection{}, Workspace{}, err
	}
	if !slices.ContainsFunc(anyOf, func(permission string) bool {
		return r.can(ctx, p, ref.OrgID, ws.OwnerType, "connection", conn.ID, permission)
	}) {
		return Connection{}, Workspace{}, ErrForbidden
	}
	return conn, ws, nil
}
