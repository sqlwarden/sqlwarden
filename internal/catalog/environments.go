package catalog

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// ListEnvironments returns the workspace environments the account may see,
// filtered, sorted, and paged.
func (s *Service) ListEnvironments(ctx context.Context, accountID, orgID, workspaceID int64, query ListQuery) (response.Paginated[database.Environment], error) {
	envs, err := s.store.AccessibleEnvironments(ctx, accountID, orgID, workspaceID)
	if err != nil {
		return response.Paginated[database.Environment]{}, err
	}
	return response.PaginateItems(filterEnvironments(envs, query), query.Page, query.PageSize), nil
}

// ListWorkspaceEnvironments returns every environment in an owner-scoped
// workspace, using the same filtering and paging contract as organization
// routes.
func (s *Service) ListWorkspaceEnvironments(ctx context.Context, workspaceID int64, query ListQuery) (response.Paginated[database.Environment], error) {
	return s.store.WorkspaceEnvironments(ctx, database.ListEnvironmentsParams{
		WorkspaceID: workspaceID, Search: query.Search, Name: query.Name,
		Sort: query.Sort, Order: query.Order, Page: query.Page, PageSize: query.PageSize,
	})
}

// Environment returns an environment only when the account may reach it under
// an organization-owned workspace.
func (s *Service) Environment(ctx context.Context, accountID, orgID int64, ws database.Workspace, env database.Environment) (database.Environment, error) {
	if ws.OwnerType != "org" {
		return env, nil
	}
	ok, err := s.store.HasAccessibleEnvironment(ctx, accountID, orgID, ws.ID, env.ID)
	if err != nil {
		return database.Environment{}, err
	}
	if !ok {
		return database.Environment{}, ErrNotFound
	}
	return env, nil
}

// CreateEnvironmentInput is a request to create a workspace environment.
type CreateEnvironmentInput struct {
	Name        string
	Description string
}

// CreateEnvironment creates an environment inside a workspace.
func (s *Service) CreateEnvironment(ctx context.Context, actor Actor, workspaceID int64, input CreateEnvironmentInput) (database.Environment, error) {
	if input.Name == "" {
		return database.Environment{}, ErrNameRequired
	}
	env, err := s.store.CreateEnvironment(ctx, workspaceID, input.Name, input.Description)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return database.Environment{}, ErrNameTaken
		}
		return database.Environment{}, err
	}
	if err := s.auditSuccess(ctx, actor, ActionEnvironmentCreated, resourceEnvironment, env.ID, map[string]string{
		"workspace_id": strconv.FormatInt(workspaceID, 10),
	}); err != nil {
		return database.Environment{}, err
	}
	return env, nil
}

// UpdateEnvironment renames an environment. Its workspace is immutable,
// because the environment's position in the resource hierarchy is what
// workspace-scoped policy is evaluated against.
func (s *Service) UpdateEnvironment(ctx context.Context, actor Actor, env database.Environment, input CreateEnvironmentInput) error {
	if input.Name == "" {
		return ErrNameRequired
	}
	if err := s.store.UpdateEnvironment(ctx, env.ID, input.Name, input.Description); err != nil {
		if database.IsUniqueViolation(err) {
			return ErrNameTaken
		}
		return err
	}
	return s.auditSuccess(ctx, actor, ActionEnvironmentUpdated, resourceEnvironment, env.ID, map[string]string{
		"workspace_id": strconv.FormatInt(env.WorkspaceID, 10),
	})
}

// DeleteEnvironment removes an empty environment. The connections tagged to it
// are read before the delete so their ancestry caches can be invalidated too:
// deleting the environment removes the hierarchy rows those cached decisions
// were derived from.
func (s *Service) DeleteEnvironment(ctx context.Context, actor Actor, env database.Environment) error {
	connectionIDs, err := s.store.ConnectionIDsByEnvironment(ctx, env.ID)
	if err != nil {
		return err
	}

	if err := s.store.DeleteEnvironment(ctx, env.ID); err != nil {
		if errors.Is(err, database.ErrEnvironmentHasConnections) {
			return ErrEnvironmentHasConnections
		}
		return err
	}

	s.grants.InvalidateAncestry(resourceEnvironment, env.ID)
	for _, connectionID := range connectionIDs {
		s.grants.InvalidateAncestry(resourceConnection, connectionID)
	}

	return s.auditSuccess(ctx, actor, ActionEnvironmentDeleted, resourceEnvironment, env.ID, map[string]string{
		"workspace_id":         strconv.FormatInt(env.WorkspaceID, 10),
		"affected_connections": strconv.Itoa(len(connectionIDs)),
	})
}

// ResolveWorkspaceEnvironment proves that an environment belongs to the
// workspace the request addressed, returning [ErrNotFound] when it does not.
// It is exported for the owner-scoped personal-space routes, which register
// connections outside organization RBAC but under the same workspace-ownership
// rule.
func (s *Service) ResolveWorkspaceEnvironment(ctx context.Context, workspaceID int64, environmentID *int64) (*int64, error) {
	return s.resolveConnectionEnvironment(ctx, workspaceID, environmentID)
}

// resolveConnectionEnvironment proves that an environment belongs to the
// workspace the request addressed before it can be used as a connection's
// environment. A nil environment means "the workspace default", which the
// store resolves at insert time.
func (s *Service) resolveConnectionEnvironment(ctx context.Context, workspaceID int64, environmentID *int64) (*int64, error) {
	if environmentID == nil {
		return nil, nil
	}
	env, found, err := s.store.Environment(ctx, *environmentID)
	if err != nil {
		return nil, err
	}
	if !found || env.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	return &env.ID, nil
}

func filterEnvironments(envs []database.Environment, query ListQuery) []database.Environment {
	filtered := make([]database.Environment, 0, len(envs))
	search := strings.ToLower(trimmed(query.Search))
	name := trimmed(query.Name)

	for _, env := range envs {
		if !matchesSearch(env.Name, search) {
			continue
		}
		if name != "" && env.Name != name {
			continue
		}
		filtered = append(filtered, env)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareEnvironment(filtered[i], filtered[j], query.Sort)
		if query.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return filtered
}

func compareEnvironment(left, right database.Environment, sortBy string) int {
	switch sortBy {
	case "name":
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}
	default:
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.Before(right.CreatedAt) {
				return -1
			}
			return 1
		}
	}
	return compareIDs(left.ID, right.ID)
}
