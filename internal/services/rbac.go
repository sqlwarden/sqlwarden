package services

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

var errRBACEnforcerRequired = errors.New("services: enforcer is required to change roles and policies")

// rbacStore adapts the metadata database and the enforcer's role and binding
// writes to the rbac service store.
type rbacStore struct {
	*database.DB
	enforcer *access.Enforcer
}

func (s rbacStore) CreateRole(ctx context.Context, orgID int64, workspaceID *int64, name, description, scopeType string, permissions []string) (int64, error) {
	if s.enforcer == nil {
		return 0, errRBACEnforcerRequired
	}
	return s.enforcer.CreateRole(ctx, orgID, workspaceID, name, description, scopeType, permissions)
}

func (s rbacStore) UpdateRole(ctx context.Context, roleID, orgID int64, name, description string, permissions []string) error {
	if s.enforcer == nil {
		return errRBACEnforcerRequired
	}
	return s.enforcer.UpdateRole(ctx, roleID, orgID, name, description, permissions)
}

func (s rbacStore) DeleteRole(ctx context.Context, roleID, orgID int64) error {
	if s.enforcer == nil {
		return errRBACEnforcerRequired
	}
	return s.enforcer.DeleteRole(ctx, roleID, orgID)
}

func (s rbacStore) BindRole(ctx context.Context, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	if s.enforcer == nil {
		return errRBACEnforcerRequired
	}
	return s.enforcer.BindRole(ctx, orgID, roleID, subjectType, subjectID, resourceType, resourceID, grantedBy)
}

func (s rbacStore) UnbindRole(ctx context.Context, bindingID, orgID int64) error {
	if s.enforcer == nil {
		return errRBACEnforcerRequired
	}
	return s.enforcer.UnbindRole(ctx, bindingID, orgID)
}
