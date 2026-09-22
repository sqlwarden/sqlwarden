package catalog

import (
	"context"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// ListWorkspaces returns the organization workspaces the account may see,
// filtered, sorted, and paged. The underlying read is already account-scoped,
// so a workspace the account cannot reach never enters the page.
func (s *Service) ListWorkspaces(ctx context.Context, accountID, orgID int64, query ListQuery) (response.Paginated[database.Workspace], error) {
	workspaces, err := s.store.AccessibleWorkspaces(ctx, accountID, orgID)
	if err != nil {
		return response.Paginated[database.Workspace]{}, err
	}
	page := response.PaginateItems(filterWorkspaces(workspaces, query), query.Page, query.PageSize)
	if err := s.store.PopulateWorkspaceCounts(ctx, page.Items); err != nil {
		return response.Paginated[database.Workspace]{}, err
	}
	return page, nil
}

// ListPersonalWorkspaces returns personal-space workspaces owned by accountID.
func (s *Service) ListPersonalWorkspaces(ctx context.Context, accountID int64, query ListQuery) (response.Paginated[database.Workspace], error) {
	return s.store.PersonalWorkspaces(ctx, database.ListWorkspacesParams{
		OwnerType: "space", OwnerID: accountID, Search: query.Search, Name: query.Name,
		Sort: query.Sort, Order: query.Order, Page: query.Page, PageSize: query.PageSize,
	})
}

// Workspace returns an organization-owned workspace only when the account can
// reach it, so an identifier from another tenant reads as absent rather than
// as forbidden. A workspace outside an organization is owner-scoped and is
// returned as given.
func (s *Service) Workspace(ctx context.Context, accountID, orgID int64, ws database.Workspace) (database.Workspace, error) {
	if ws.OwnerType == "org" {
		ok, err := s.store.HasAccessibleWorkspace(ctx, accountID, orgID, ws.ID)
		if err != nil {
			return database.Workspace{}, err
		}
		if !ok {
			return database.Workspace{}, ErrNotFound
		}
	}
	workspaces := []database.Workspace{ws}
	if err := s.store.PopulateWorkspaceCounts(ctx, workspaces); err != nil {
		return database.Workspace{}, err
	}
	return workspaces[0], nil
}

// CreateWorkspaceInput is a request to create an organization workspace.
type CreateWorkspaceInput struct {
	Name        string
	Description string
}

// CreateWorkspace creates a workspace with its hierarchy row, default
// environment, and seeded builtin workspace roles and policies in one
// transaction, then drops the authorization caches the new bindings affect.
func (s *Service) CreateWorkspace(ctx context.Context, actor Actor, orgID int64, input CreateWorkspaceInput) (database.Workspace, error) {
	if input.Name == "" {
		return database.Workspace{}, ErrNameRequired
	}

	ws, err := s.store.CreateWorkspace(ctx, orgID, actor.AccountID, input.Name, input.Description)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return database.Workspace{}, ErrNameTaken
		}
		return database.Workspace{}, err
	}

	s.grants.InvalidateOrgPolicy(orgID)
	s.grants.InvalidatePrincipals(orgID, actor.AccountID)

	workspaces := []database.Workspace{ws}
	if err := s.store.PopulateWorkspaceCounts(ctx, workspaces); err != nil {
		return database.Workspace{}, err
	}
	ws = workspaces[0]

	if err := s.auditSuccess(ctx, actor, ActionWorkspaceCreated, resourceWorkspace, ws.ID, nil); err != nil {
		return database.Workspace{}, err
	}
	return ws, nil
}

// CreatePersonalWorkspace creates an owner-scoped workspace. Personal spaces
// do not seed organization policy because their owner has unconditional access.
func (s *Service) CreatePersonalWorkspace(ctx context.Context, actor Actor, input CreateWorkspaceInput) (database.Workspace, error) {
	if input.Name == "" {
		return database.Workspace{}, ErrNameRequired
	}
	ws, err := s.store.CreatePersonalWorkspace(ctx, actor.AccountID, input.Name, input.Description)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return database.Workspace{}, ErrNameTaken
		}
		return database.Workspace{}, err
	}
	if err := s.auditSuccess(ctx, actor, ActionWorkspaceCreated, resourceWorkspace, ws.ID, map[string]string{"owner_type": "space"}); err != nil {
		return database.Workspace{}, err
	}
	return ws, nil
}

// UpdateWorkspace renames a workspace. Ownership is immutable: a workspace
// cannot be moved between organizations or owners, because its seeded policy
// and hierarchy rows are written for the owner it was created under.
func (s *Service) UpdateWorkspace(ctx context.Context, actor Actor, ws database.Workspace, input CreateWorkspaceInput) error {
	if input.Name == "" {
		return ErrNameRequired
	}
	if err := s.store.UpdateWorkspace(ctx, ws.ID, input.Name, input.Description); err != nil {
		return err
	}
	return s.auditSuccess(ctx, actor, ActionWorkspaceUpdated, resourceWorkspace, ws.ID, nil)
}

// DeleteWorkspace removes a workspace and invalidates the ancestry cached for
// it, so a later authorization decision cannot be made from a hierarchy that
// no longer exists.
func (s *Service) DeleteWorkspace(ctx context.Context, actor Actor, ws database.Workspace) error {
	if err := s.store.DeleteWorkspace(ctx, ws.ID); err != nil {
		return err
	}
	s.grants.InvalidateAncestry(resourceWorkspace, ws.ID)
	return s.auditSuccess(ctx, actor, ActionWorkspaceDeleted, resourceWorkspace, ws.ID, nil)
}

func filterWorkspaces(workspaces []database.Workspace, query ListQuery) []database.Workspace {
	filtered := make([]database.Workspace, 0, len(workspaces))
	search := strings.ToLower(trimmed(query.Search))
	name := trimmed(query.Name)

	for _, workspace := range workspaces {
		if !matchesSearch(workspace.Name, search) {
			continue
		}
		if name != "" && workspace.Name != name {
			continue
		}
		filtered = append(filtered, workspace)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareWorkspace(filtered[i], filtered[j], query.Sort)
		if query.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return filtered
}

func compareWorkspace(left, right database.Workspace, sortBy string) int {
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

func compareIDs(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
