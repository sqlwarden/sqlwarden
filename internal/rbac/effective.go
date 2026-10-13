package rbac

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/access"
)

// ResourceRef names the resource whose effective permissions are requested,
// as raw query values. An empty Type means the organization, and an empty ID
// on the organization means the requested organization.
type ResourceRef struct {
	Type string
	ID   string
}

// EffectivePermissions is the set of permissions the caller holds on one
// resource. Fields are declared in JSON key order.
type EffectivePermissions struct {
	ResourceType string   `json:"resource_type"`
	ResourceID   int64    `json:"resource_id"`
	Permissions  []string `json:"permissions"`
}

// ResourceRejectedError reports a requested resource outside the organization.
// It wraps ErrNotFound; Reason names the failed check for logging.
type ResourceRejectedError struct {
	ResourceType string
	ResourceID   int64
	Reason       string
}

func (e *ResourceRejectedError) Error() string { return ErrNotFound.Error() }
func (e *ResourceRejectedError) Unwrap() error { return ErrNotFound }

// EffectivePermissions returns the permissions the caller holds on an
// organization, workspace, environment, or connection, including those
// inherited from enclosing resources. Any organization member may ask about
// their own permissions; a resource outside the organization is reported as a
// ResourceRejectedError.
func (s *Service) EffectivePermissions(ctx context.Context, p access.Principal, orgID int64, ref ResourceRef) (EffectivePermissions, error) {
	if err := s.requireOrg(ctx, p, orgID, ""); err != nil {
		return EffectivePermissions{}, err
	}
	resourceType := strings.TrimSpace(ref.Type)
	if resourceType == "" {
		resourceType = "org"
	}
	resourceID, err := s.resolveEffectiveResource(ctx, orgID, resourceType, strings.TrimSpace(ref.ID))
	if err != nil {
		return EffectivePermissions{}, err
	}
	if s.policy == nil {
		return EffectivePermissions{}, errors.New("rbac: policy evaluator is required for effective permissions")
	}
	permissions, err := s.policy.EffectivePermissions(ctx, p.Subject.ID, orgID, "org", resourceType, resourceID)
	if err != nil {
		return EffectivePermissions{}, err
	}
	return EffectivePermissions{ResourceType: resourceType, ResourceID: resourceID, Permissions: permissions}, nil
}

func (s *Service) resolveEffectiveResource(ctx context.Context, orgID int64, resourceType, rawID string) (int64, error) {
	switch resourceType {
	case "org", "workspace", "environment", "connection":
	default:
		return 0, &ValidationError{Fields: map[string]string{"resource_type": "Resource type must be org, workspace, environment, or connection."}}
	}
	if resourceType == "org" && rawID == "" {
		return orgID, nil
	}
	resourceID, err := parseResourceID(rawID)
	if err != nil {
		return 0, err
	}
	reject := func(reason string) error {
		return &ResourceRejectedError{ResourceType: resourceType, ResourceID: resourceID, Reason: reason}
	}

	var workspaceID int64
	switch resourceType {
	case "org":
		if resourceID != orgID {
			return 0, reject("org_mismatch")
		}
		return resourceID, nil
	case "workspace":
		workspaceID = resourceID
	case "environment":
		env, found, err := s.store.GetEnvironment(ctx, resourceID)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, reject("environment_not_found")
		}
		workspaceID = env.WorkspaceID
	case "connection":
		conn, found, err := s.store.GetConnection(ctx, resourceID)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, reject("connection_not_found")
		}
		workspaceID = conn.WorkspaceID
	}
	ws, found, err := s.store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	if !found || ws.OrgID == nil || *ws.OrgID != orgID {
		if resourceType == "workspace" {
			return 0, reject("workspace_not_in_org")
		}
		return 0, reject(resourceType + "_workspace_not_in_org")
	}
	return resourceID, nil
}

func parseResourceID(raw string) (int64, error) {
	if raw == "" {
		return 0, &ValidationError{Fields: map[string]string{"resource_id": "Resource is required."}}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, &ValidationError{Fields: map[string]string{"resource_id": "Resource must be a positive integer."}}
	}
	return id, nil
}
