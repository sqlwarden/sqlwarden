package orgs

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Team is an organization team as the service returns it.
type Team = database.Team

// TeamMember is a team member with its account details.
type TeamMember = database.TeamMemberListItem

// ListTeamsParams filters and pages a team list.
type ListTeamsParams = database.ListTeamsParams

// ListTeamMembersParams filters and pages a team member list.
type ListTeamMembersParams = database.ListTeamMembersParams

// ListTeams returns a page of the organization's teams. params.OrgID is set
// from orgID.
func (s *Service) ListTeams(ctx context.Context, p access.Principal, orgID int64, params ListTeamsParams) (response.Paginated[Team], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return response.Paginated[Team]{}, err
	}
	params.OrgID = orgID
	return s.teams.ListTeamsPage(ctx, params)
}

type CreateTeamInput struct {
	Slug string
	Name string
}

// CreateTeam creates a team in the organization. Slugs are unique within an
// organization.
func (s *Service) CreateTeam(ctx context.Context, p access.Principal, orgID int64, in CreateTeamInput) (Team, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Team{}, err
	}
	fields := map[string]string{}
	if in.Name == "" {
		addField(fields, "name", "Name is required.")
	}
	if in.Slug == "" {
		addField(fields, "slug", "Slug is required.")
	} else if !ValidSlug(in.Slug) {
		addField(fields, "slug", "Slug may only contain lowercase letters, numbers, and hyphens.")
	}
	if len(fields) != 0 {
		return Team{}, &ValidationError{Fields: fields}
	}

	team, err := s.teams.InsertTeam(ctx, orgID, in.Slug, in.Name)
	if err != nil {
		if isUniqueViolation(err) {
			return Team{}, &ValidationError{Fields: map[string]string{"slug": "A team with this slug already exists in this organization."}}
		}
		return Team{}, err
	}
	return team, nil
}

// GetTeam returns the organization's team with slug. A team of another
// organization is reported as ErrNotFound.
func (s *Service) GetTeam(ctx context.Context, p access.Principal, orgID int64, slug string) (Team, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return Team{}, err
	}
	return s.loadTeam(ctx, orgID, slug)
}

// UpdateTeamInput mirrors the update request. Slug and OrgID are immutable;
// supplying either is a validation failure.
type UpdateTeamInput struct {
	Name  string
	Slug  *string
	OrgID *int64
}

// UpdateTeam renames the team.
func (s *Service) UpdateTeam(ctx context.Context, p access.Principal, orgID int64, slug string, in UpdateTeamInput) (Team, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Team{}, err
	}
	fields := map[string]string{}
	if in.Name == "" {
		addField(fields, "name", "Name is required.")
	}
	if in.Slug != nil {
		addField(fields, "slug", "Slug is immutable.")
	}
	if in.OrgID != nil {
		addField(fields, "org_id", "Organization is immutable.")
	}
	if len(fields) != 0 {
		return Team{}, &ValidationError{Fields: fields}
	}

	team, err := s.loadTeam(ctx, orgID, slug)
	if err != nil {
		return Team{}, err
	}
	if err := s.teams.UpdateTeam(ctx, team.ID, orgID, in.Name); err != nil {
		return Team{}, err
	}
	team.Name = in.Name
	return team, nil
}

// DeleteTeam deletes the team together with the role bindings granted to it.
func (s *Service) DeleteTeam(ctx context.Context, p access.Principal, orgID int64, slug string) (Team, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Team{}, err
	}
	team, err := s.loadTeam(ctx, orgID, slug)
	if err != nil {
		return Team{}, err
	}
	if err := s.teams.DeleteTeam(ctx, team.ID, orgID); err != nil {
		return Team{}, err
	}
	s.invalidateOrgPolicy(orgID)
	return team, nil
}

// ListTeamMembers returns a page of the team's members. params.TeamID is set
// from the resolved team.
func (s *Service) ListTeamMembers(ctx context.Context, p access.Principal, orgID int64, slug string, params ListTeamMembersParams) (response.Paginated[TeamMember], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return response.Paginated[TeamMember]{}, err
	}
	team, err := s.loadTeam(ctx, orgID, slug)
	if err != nil {
		return response.Paginated[TeamMember]{}, err
	}
	params.TeamID = team.ID
	return s.teams.ListTeamMembersPage(ctx, params)
}

// AddTeamMember adds an organization member to the team. Accounts outside the
// organization are reported as ErrNotFound; adding an existing member
// succeeds without effect.
func (s *Service) AddTeamMember(ctx context.Context, p access.Principal, orgID int64, slug string, accountID int64) (Team, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Team{}, err
	}
	if accountID <= 0 {
		return Team{}, &ValidationError{Fields: map[string]string{"account_id": "Account is required."}}
	}
	team, err := s.loadTeam(ctx, orgID, slug)
	if err != nil {
		return Team{}, err
	}
	if err := s.requireMember(ctx, orgID, accountID); err != nil {
		return Team{}, err
	}
	if err := s.teams.AddTeamMember(ctx, team.ID, accountID); err != nil {
		switch {
		case isUniqueViolation(err):
			return team, nil
		case isForeignKeyViolation(err):
			return Team{}, ErrNotFound
		default:
			return Team{}, err
		}
	}
	s.invalidatePrincipals(orgID, accountID)
	return team, nil
}

// RemoveTeamMember removes the account from the team and closes its live
// sessions in every workspace the team grants access to. It returns the team
// and the number of those workspaces.
func (s *Service) RemoveTeamMember(ctx context.Context, p access.Principal, orgID int64, slug string, accountID int64) (Team, int, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Team{}, 0, err
	}
	team, err := s.loadTeam(ctx, orgID, slug)
	if err != nil {
		return Team{}, 0, err
	}
	if err := s.teams.RemoveTeamMember(ctx, team.ID, accountID); err != nil {
		return Team{}, 0, err
	}
	workspaceIDs, err := s.teams.ListWorkspaceIDsForTeam(ctx, team.ID)
	if err != nil {
		return Team{}, 0, err
	}
	for _, workspaceID := range workspaceIDs {
		s.revokeWorkspaceSessions(ctx, workspaceID, accountID)
	}
	s.invalidatePrincipals(orgID, accountID)
	return team, len(workspaceIDs), nil
}

func (s *Service) loadTeam(ctx context.Context, orgID int64, slug string) (Team, error) {
	team, found, err := s.teams.GetTeam(ctx, orgID, slug)
	if err != nil {
		return Team{}, err
	}
	if !found {
		return Team{}, ErrNotFound
	}
	return team, nil
}

// revokeWorkspaceSessions runs after the membership change committed; see
// revokeSessions.
func (s *Service) revokeWorkspaceSessions(ctx context.Context, workspaceID, accountID int64) {
	if s.revoker == nil {
		return
	}
	if _, err := s.revoker.RevokeWorkspaceAccount(context.WithoutCancel(ctx), strconv.FormatInt(workspaceID, 10), strconv.FormatInt(accountID, 10)); err != nil {
		s.logger.WarnContext(ctx, "team member workspace session revocation failed", slog.Int64("workspace_id", workspaceID), slog.Int64("target_account_id", accountID), slog.Any("error", err))
	}
}
