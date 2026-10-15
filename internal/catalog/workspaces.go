package catalog

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Workspace is an organization workspace as the service returns it.
type Workspace = database.Workspace

const (
	ActionWorkspaceCreated = "workspace.created"
	ActionWorkspaceDeleted = "workspace.deleted"
)

// WorkspaceStore is the workspace, environment, and workspace membership
// metadata the service reads and writes.
type WorkspaceStore interface {
	ListAccessibleWorkspaces(ctx context.Context, accountID, orgID int64) ([]database.Workspace, error)
	HasAccessibleWorkspace(ctx context.Context, accountID, orgID, workspaceID int64) (bool, error)
	PopulateWorkspaceCounts(ctx context.Context, workspaces []database.Workspace) error
	// CreateOwnedWorkspace inserts the workspace with its hierarchy rows and
	// default environment, and seeds its builtin roles with the creator bound
	// as administrator, in one transaction.
	CreateOwnedWorkspace(ctx context.Context, orgID, creatorAccountID int64, name, description string) (database.Workspace, error)
	UpdateWorkspace(ctx context.Context, id int64, name, description string) error
	DeleteWorkspace(ctx context.Context, id int64) error

	ListAccessibleEnvironments(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Environment, error)
	HasAccessibleEnvironment(ctx context.Context, accountID, orgID, workspaceID, environmentID int64) (bool, error)
	// InsertEnvironment inserts the environment with its hierarchy rows.
	InsertEnvironment(ctx context.Context, workspaceID int64, name, description string) (database.Environment, error)
	UpdateEnvironment(ctx context.Context, id int64, name, description string) error
	ListConnectionIDsByEnvironment(ctx context.Context, environmentID int64) ([]int64, error)
	ListAccessibleConnections(ctx context.Context, accountID, orgID, workspaceID int64) ([]database.Connection, error)
	// DeleteEnvironment refuses an environment that still holds connections
	// with database.ErrEnvironmentHasConnections.
	DeleteEnvironment(ctx context.Context, id int64) error

	ListWorkspaceMembersPage(ctx context.Context, params database.ListWorkspaceMembersParams) (response.Paginated[database.WorkspaceMemberListItem], error)
	ListWorkspaceEffectiveMembersPage(ctx context.Context, orgID int64, params database.ListWorkspaceMembersParams) (response.Paginated[database.WorkspaceEffectiveMemberListItem], error)
	AddWorkspaceMember(ctx context.Context, workspaceID, accountID int64, createdBy *int64) error
	RemoveWorkspaceMember(ctx context.Context, workspaceID, accountID int64) error
	ListWorkspaceTeamsPage(ctx context.Context, params database.ListWorkspaceTeamsParams) (response.Paginated[database.WorkspaceTeamListItem], error)
	GetTeamByID(ctx context.Context, id int64) (database.Team, bool, error)
	AddWorkspaceTeam(ctx context.Context, workspaceID, teamID int64, createdBy *int64) error
	RemoveWorkspaceTeam(ctx context.Context, workspaceID, teamID int64) error
	ListWorkspaceTeamMemberAccountIDs(ctx context.Context, workspaceID, teamID int64) ([]int64, error)
}

// ListQuery filters, sorts, and pages a workspace or environment list. Sort
// is "name" or "created_at"; Order is "asc" or "desc".
type ListQuery struct {
	Search   string
	Name     string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// ListWorkspaces returns a page of the organization's workspaces the
// principal can read, with their environment and connection counts.
func (s *Service) ListWorkspaces(ctx context.Context, p access.Principal, orgID int64, q ListQuery) (response.Paginated[Workspace], error) {
	if _, err := s.requirer().Org(ctx, p, orgID); err != nil {
		return response.Paginated[Workspace]{}, err
	}
	workspaces, err := s.deps.Workspaces.ListAccessibleWorkspaces(ctx, p.Subject.ID, orgID)
	if err != nil {
		return response.Paginated[Workspace]{}, err
	}
	workspaces = filterAndSort(workspaces, q, func(ws Workspace) (string, time.Time, int64) { return ws.Name, ws.CreatedAt, ws.ID })
	page := response.PaginateItems(workspaces, q.Page, q.PageSize)
	if err := s.deps.Workspaces.PopulateWorkspaceCounts(ctx, page.Items); err != nil {
		return response.Paginated[Workspace]{}, err
	}
	return page, nil
}

type CreateWorkspaceInput struct {
	Name        string
	Description string
}

// CreateWorkspace creates an organization workspace and makes the calling
// account its administrator.
func (s *Service) CreateWorkspace(ctx context.Context, p access.Principal, orgID int64, in CreateWorkspaceInput) (Workspace, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermWsCreate); err != nil {
		return Workspace{}, err
	}
	if in.Name == "" {
		return Workspace{}, validationError("name", "Name is required.")
	}
	ws, err := s.deps.Workspaces.CreateOwnedWorkspace(ctx, orgID, p.Subject.ID, in.Name, in.Description)
	if err != nil {
		if isUniqueViolation(err) {
			return Workspace{}, validationError("name", "A workspace with this name already exists in this organization.")
		}
		return Workspace{}, err
	}
	if s.deps.Cache != nil {
		s.deps.Cache.InvalidateOrgPolicy(orgID)
		s.deps.Cache.InvalidatePrincipals(orgID, p.Subject.ID)
	}
	s.emitWorkspaceEvent(ctx, p, orgID, ActionWorkspaceCreated, ws.ID)

	workspaces := []Workspace{ws}
	if err := s.deps.Workspaces.PopulateWorkspaceCounts(ctx, workspaces); err != nil {
		return Workspace{}, err
	}
	return workspaces[0], nil
}

// GetWorkspace returns the workspace when the principal can read it. A
// workspace the principal cannot read is reported as ErrNotFound.
func (s *Service) GetWorkspace(ctx context.Context, p access.Principal, orgID, workspaceID int64) (Workspace, error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, ""); err != nil {
		return Workspace{}, err
	}
	ok, err := s.deps.Workspaces.HasAccessibleWorkspace(ctx, p.Subject.ID, orgID, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if !ok {
		return Workspace{}, ErrNotFound
	}
	ws, found, err := s.deps.Store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return Workspace{}, err
	}
	if !found {
		return Workspace{}, ErrNotFound
	}
	workspaces := []Workspace{ws}
	if err := s.deps.Workspaces.PopulateWorkspaceCounts(ctx, workspaces); err != nil {
		return Workspace{}, err
	}
	return workspaces[0], nil
}

// UpdateWorkspaceInput mirrors the update request. The organization and
// owner are immutable; supplying any of them is a validation failure.
type UpdateWorkspaceInput struct {
	Name        string
	Description string
	OrgID       *int64
	OwnerType   *string
	OwnerID     *int64
}

// UpdateWorkspace renames the workspace and replaces its description.
func (s *Service) UpdateWorkspace(ctx context.Context, p access.Principal, orgID, workspaceID int64, in UpdateWorkspaceInput) error {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermWsWrite); err != nil {
		return err
	}
	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "Name is required."
	}
	if in.OrgID != nil {
		fields["org_id"] = "Organization is immutable."
	}
	if in.OwnerType != nil {
		fields["owner_type"] = "Owner type is immutable."
	}
	if in.OwnerID != nil {
		fields["owner_id"] = "Owner is immutable."
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}
	return s.deps.Workspaces.UpdateWorkspace(ctx, workspaceID, in.Name, in.Description)
}

// DeleteWorkspace deletes the workspace and everything it owns.
func (s *Service) DeleteWorkspace(ctx context.Context, p access.Principal, orgID, workspaceID int64) error {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermWsDelete); err != nil {
		return err
	}
	if err := s.deps.Workspaces.DeleteWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	if s.deps.Ancestry != nil {
		s.deps.Ancestry.InvalidateAncestry("workspace", workspaceID)
	}
	s.emitWorkspaceEvent(ctx, p, orgID, ActionWorkspaceDeleted, workspaceID)
	return nil
}

// requireOrg admits org members who hold permission on the organization.
func (s *Service) requireOrg(ctx context.Context, p access.Principal, orgID int64, permission string) error {
	requirer := s.requirer()
	if _, err := requirer.Org(ctx, p, orgID); err != nil {
		return err
	}
	if !requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
		return ErrForbidden
	}
	return nil
}

// emitWorkspaceEvent records a success event on a best-effort basis; a failed
// audit write must not undo or fail a change that already committed.
func (s *Service) emitWorkspaceEvent(ctx context.Context, p access.Principal, orgID int64, action string, workspaceID int64) {
	if s.deps.Audit == nil {
		return
	}
	err := audit.Emit(ctx, s.deps.Audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     action,
		Resource:   "workspace",
		ResourceID: strconv.FormatInt(workspaceID, 10),
		Outcome:    audit.OutcomeSuccess,
	})
	if err != nil {
		s.deps.Logger.WarnContext(ctx, "workspace audit failed", slog.String("action", action), slog.Int64("org_id", orgID), slog.Int64("workspace_id", workspaceID), slog.Any("error", err))
	}
}

// filterAndSort keeps the items whose name contains q.Search (case
// insensitive) and equals q.Name, then orders them by name or creation time
// with the id as tiebreaker.
func filterAndSort[T any](items []T, q ListQuery, key func(T) (string, time.Time, int64)) []T {
	search := strings.ToLower(strings.TrimSpace(q.Search))
	name := strings.TrimSpace(q.Name)
	filtered := make([]T, 0, len(items))
	for _, item := range items {
		itemName, _, _ := key(item)
		if search != "" && !strings.Contains(strings.ToLower(itemName), search) {
			continue
		}
		if name != "" && itemName != name {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareListItems(q.Sort, filtered[i], filtered[j], key)
		if q.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	return filtered
}

func compareListItems[T any](sortBy string, left, right T, key func(T) (string, time.Time, int64)) int {
	leftName, leftCreated, leftID := key(left)
	rightName, rightCreated, rightID := key(right)
	switch sortBy {
	case "name":
		if leftName != rightName {
			return strings.Compare(leftName, rightName)
		}
	default:
		if !leftCreated.Equal(rightCreated) {
			if leftCreated.Before(rightCreated) {
				return -1
			}
			return 1
		}
	}
	switch {
	case leftID < rightID:
		return -1
	case leftID > rightID:
		return 1
	default:
		return 0
	}
}
