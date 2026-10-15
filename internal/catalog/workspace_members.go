package catalog

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

type (
	WorkspaceMember          = database.WorkspaceMemberListItem
	WorkspaceEffectiveMember = database.WorkspaceEffectiveMemberListItem
	WorkspaceTeam            = database.WorkspaceTeamListItem

	// ListWorkspaceMembersParams filters and pages a workspace member list.
	ListWorkspaceMembersParams = database.ListWorkspaceMembersParams
	// ListWorkspaceTeamsParams filters and pages a workspace team list.
	ListWorkspaceTeamsParams = database.ListWorkspaceTeamsParams
)

// ListWorkspaceMembers returns a page of the workspace's direct members.
// params.WorkspaceID is set from workspaceID.
func (s *Service) ListWorkspaceMembers(ctx context.Context, p access.Principal, orgID, workspaceID int64, params ListWorkspaceMembersParams) (response.Paginated[WorkspaceMember], error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return response.Paginated[WorkspaceMember]{}, err
	}
	params.WorkspaceID = workspaceID
	return s.deps.Workspaces.ListWorkspaceMembersPage(ctx, params)
}

// ListWorkspaceEffectiveMembers returns a page of the accounts that belong to
// the workspace directly or through a workspace team. params.WorkspaceID is
// set from workspaceID.
func (s *Service) ListWorkspaceEffectiveMembers(ctx context.Context, p access.Principal, orgID, workspaceID int64, params ListWorkspaceMembersParams) (response.Paginated[WorkspaceEffectiveMember], error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return response.Paginated[WorkspaceEffectiveMember]{}, err
	}
	params.WorkspaceID = workspaceID
	return s.deps.Workspaces.ListWorkspaceEffectiveMembersPage(ctx, orgID, params)
}

// AddWorkspaceMember adds an organization member to the workspace. Accounts
// outside the organization are reported as ErrNotFound.
func (s *Service) AddWorkspaceMember(ctx context.Context, p access.Principal, orgID, workspaceID, accountID int64) error {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return err
	}
	if accountID <= 0 {
		return validationError("account_id", "Account is required.")
	}
	isMember, err := s.deps.Store.IsOrgMember(ctx, orgID, accountID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotFound
	}
	actorID := p.Subject.ID
	if err := s.deps.Workspaces.AddWorkspaceMember(ctx, workspaceID, accountID, &actorID); err != nil {
		return err
	}
	s.invalidatePrincipals(orgID, accountID)
	return nil
}

// RemoveWorkspaceMember removes the account's direct workspace membership and
// closes its live sessions in the workspace.
func (s *Service) RemoveWorkspaceMember(ctx context.Context, p access.Principal, orgID, workspaceID, accountID int64) error {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return err
	}
	if err := s.deps.Workspaces.RemoveWorkspaceMember(ctx, workspaceID, accountID); err != nil {
		return err
	}
	s.revokeWorkspaceSessions(ctx, workspaceID, accountID)
	s.invalidatePrincipals(orgID, accountID)
	return nil
}

// ListWorkspaceTeams returns a page of the teams granted the workspace.
// params.WorkspaceID is set from workspaceID.
func (s *Service) ListWorkspaceTeams(ctx context.Context, p access.Principal, orgID, workspaceID int64, params ListWorkspaceTeamsParams) (response.Paginated[WorkspaceTeam], error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyRead); err != nil {
		return response.Paginated[WorkspaceTeam]{}, err
	}
	params.WorkspaceID = workspaceID
	return s.deps.Workspaces.ListWorkspaceTeamsPage(ctx, params)
}

// AddWorkspaceTeam grants the organization's team the workspace and returns
// the number of team members affected. Live sessions of those members in the
// workspace are closed so they reopen under the new grants. A team of another
// organization is reported as ErrNotFound.
func (s *Service) AddWorkspaceTeam(ctx context.Context, p access.Principal, orgID, workspaceID, teamID int64) (int, error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return 0, err
	}
	if teamID <= 0 {
		return 0, validationError("team_id", "Team is required.")
	}
	team, found, err := s.deps.Workspaces.GetTeamByID(ctx, teamID)
	if err != nil {
		return 0, err
	}
	if !found || team.OrgID != orgID {
		return 0, ErrNotFound
	}
	actorID := p.Subject.ID
	if err := s.deps.Workspaces.AddWorkspaceTeam(ctx, workspaceID, teamID, &actorID); err != nil {
		return 0, err
	}
	accountIDs, err := s.deps.Workspaces.ListWorkspaceTeamMemberAccountIDs(ctx, workspaceID, teamID)
	if err != nil {
		return 0, err
	}
	for _, accountID := range accountIDs {
		s.revokeWorkspaceSessions(ctx, workspaceID, accountID)
		s.invalidatePrincipals(orgID, accountID)
	}
	return len(accountIDs), nil
}

// RemoveWorkspaceTeam revokes the team's workspace grant, closes its members'
// live sessions in the workspace, and returns the number of members affected.
func (s *Service) RemoveWorkspaceTeam(ctx context.Context, p access.Principal, orgID, workspaceID, teamID int64) (int, error) {
	if _, err := s.requirer().Workspace(ctx, p, orgID, workspaceID, access.PermPolicyModify); err != nil {
		return 0, err
	}
	// Membership through the team is gone after the removal, so the affected
	// accounts are collected first.
	accountIDs, err := s.deps.Workspaces.ListWorkspaceTeamMemberAccountIDs(ctx, workspaceID, teamID)
	if err != nil {
		return 0, err
	}
	if err := s.deps.Workspaces.RemoveWorkspaceTeam(ctx, workspaceID, teamID); err != nil {
		return 0, err
	}
	for _, accountID := range accountIDs {
		s.revokeWorkspaceSessions(ctx, workspaceID, accountID)
		s.invalidatePrincipals(orgID, accountID)
	}
	return len(accountIDs), nil
}

func (s *Service) invalidatePrincipals(orgID, accountID int64) {
	if s.deps.Cache != nil {
		s.deps.Cache.InvalidatePrincipals(orgID, accountID)
	}
}

// revokeWorkspaceSessions runs after the membership change committed. The
// revoker queues a retry on failure, so an error here never fails the change.
func (s *Service) revokeWorkspaceSessions(ctx context.Context, workspaceID, accountID int64) {
	if s.deps.Revoker == nil {
		return
	}
	if _, err := s.deps.Revoker.RevokeWorkspaceAccount(context.WithoutCancel(ctx), strconv.FormatInt(workspaceID, 10), strconv.FormatInt(accountID, 10)); err != nil {
		s.deps.Logger.WarnContext(ctx, "workspace session revocation failed", slog.Int64("workspace_id", workspaceID), slog.Int64("target_account_id", accountID), slog.Any("error", err))
	}
}
