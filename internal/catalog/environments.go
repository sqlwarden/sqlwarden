package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Environment is a workspace environment as the service returns it.
type Environment = database.Environment

// ListEnvironments returns a page of the workspace's environments the
// principal can read.
func (s *Service) ListEnvironments(ctx context.Context, p access.Principal, orgID, workspaceID int64, q ListQuery) (response.Paginated[Environment], error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, ""); err != nil {
		return response.Paginated[Environment]{}, err
	}
	envs, err := s.deps.Workspaces.ListAccessibleEnvironments(ctx, p.Subject.ID, orgID, workspaceID)
	if err != nil {
		return response.Paginated[Environment]{}, err
	}
	envs = filterAndSort(envs, q, func(env Environment) (string, time.Time, int64) { return env.Name, env.CreatedAt, env.ID })
	return response.PaginateItems(envs, q.Page, q.PageSize), nil
}

type EnvironmentInput struct {
	Name        string
	Description string
}

// CreateEnvironment creates an environment in the workspace.
func (s *Service) CreateEnvironment(ctx context.Context, p access.Principal, orgID, workspaceID int64, in EnvironmentInput) (Environment, error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermEnvCreate); err != nil {
		return Environment{}, err
	}
	if in.Name == "" {
		return Environment{}, validationError("name", "Name is required.")
	}
	env, err := s.deps.Workspaces.InsertEnvironment(ctx, workspaceID, in.Name, in.Description)
	if err != nil {
		if isUniqueViolation(err) {
			return Environment{}, duplicateEnvironmentName()
		}
		return Environment{}, err
	}
	return env, nil
}

// GetEnvironment returns the workspace's environment when the principal can
// read it. An environment the principal cannot read is reported as
// ErrNotFound.
func (s *Service) GetEnvironment(ctx context.Context, p access.Principal, orgID, workspaceID, environmentID int64) (Environment, error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, ""); err != nil {
		return Environment{}, err
	}
	env, err := s.loadEnvironment(ctx, workspaceID, environmentID)
	if err != nil {
		return Environment{}, err
	}
	ok, err := s.deps.Workspaces.HasAccessibleEnvironment(ctx, p.Subject.ID, orgID, workspaceID, environmentID)
	if err != nil {
		return Environment{}, err
	}
	if !ok {
		return Environment{}, ErrNotFound
	}
	return env, nil
}

// UpdateEnvironmentInput mirrors the update request. The workspace is
// immutable; supplying it is a validation failure.
type UpdateEnvironmentInput struct {
	Name        string
	Description string
	WorkspaceID *int64
}

// UpdateEnvironment renames the environment and replaces its description.
func (s *Service) UpdateEnvironment(ctx context.Context, p access.Principal, orgID, workspaceID, environmentID int64, in UpdateEnvironmentInput) error {
	if _, err := s.requireEnvironment(ctx, p, orgID, workspaceID, environmentID, access.PermEnvWrite); err != nil {
		return err
	}
	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "Name is required."
	}
	if in.WorkspaceID != nil {
		fields["workspace_id"] = "Workspace is immutable."
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}
	if err := s.deps.Workspaces.UpdateEnvironment(ctx, environmentID, in.Name, in.Description); err != nil {
		if isUniqueViolation(err) {
			return duplicateEnvironmentName()
		}
		return err
	}
	return nil
}

// DeleteEnvironment deletes an environment that holds no connections and
// returns the number of connections whose cached ancestry it invalidated.
// An environment with connections is refused with
// ErrEnvironmentHasConnections.
func (s *Service) DeleteEnvironment(ctx context.Context, p access.Principal, orgID, workspaceID, environmentID int64) (int, error) {
	if _, err := s.requireEnvironment(ctx, p, orgID, workspaceID, environmentID, access.PermEnvDelete); err != nil {
		return 0, err
	}
	// Connection ancestry rows through this environment go away with it, so
	// the ids are collected before the delete.
	connectionIDs, err := s.deps.Workspaces.ListConnectionIDsByEnvironment(ctx, environmentID)
	if err != nil {
		return 0, err
	}
	if err := s.deps.Workspaces.DeleteEnvironment(ctx, environmentID); err != nil {
		if errors.Is(err, database.ErrEnvironmentHasConnections) {
			return 0, ErrEnvironmentHasConnections
		}
		return 0, err
	}
	if s.deps.Ancestry != nil {
		s.deps.Ancestry.InvalidateAncestry("environment", environmentID)
		for _, id := range connectionIDs {
			s.deps.Ancestry.InvalidateAncestry("connection", id)
		}
	}
	return len(connectionIDs), nil
}

// requireEnvironment resolves the workspace's environment and requires
// permission on it. An environment of another workspace is ErrNotFound.
func (s *Service) requireEnvironment(ctx context.Context, p access.Principal, orgID, workspaceID, environmentID int64, permission string) (Environment, error) {
	requirer := s.requirer()
	ws, err := requirer.Workspace(ctx, p, orgID, workspaceID, "")
	if err != nil {
		return Environment{}, err
	}
	env, err := s.loadEnvironment(ctx, workspaceID, environmentID)
	if err != nil {
		return Environment{}, err
	}
	if !requirer.Can(ctx, p, orgID, ws.OwnerType, "environment", env.ID, permission) {
		return Environment{}, ErrForbidden
	}
	return env, nil
}

func (s *Service) loadEnvironment(ctx context.Context, workspaceID, environmentID int64) (Environment, error) {
	env, found, err := s.deps.Store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return Environment{}, err
	}
	if !found || env.WorkspaceID != workspaceID {
		return Environment{}, ErrNotFound
	}
	return env, nil
}

func duplicateEnvironmentName() error {
	return validationError("name", "An environment with this name already exists in this workspace.")
}
