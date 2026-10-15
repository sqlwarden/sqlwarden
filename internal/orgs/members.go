package orgs

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/platform/invalidate"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/response"
)

// Member is an organization member with the builtin role it holds.
type Member = database.OrgMemberListItem

// ListMembersParams filters and pages a member list.
type ListMembersParams = database.ListOrgMembersParams

// MemberRemovalReason is recorded on access revoked by a membership removal.
const MemberRemovalReason = "org_membership_removed"

// ValidMemberRole reports whether role names one of the builtin organization
// roles a member can be assigned.
func ValidMemberRole(role string) bool {
	switch role {
	case access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, access.BuiltinOrgMemberRole:
		return true
	default:
		return false
	}
}

// ListMembers returns a page of the organization's members. params.OrgID is
// set from orgID.
func (s *Service) ListMembers(ctx context.Context, p access.Principal, orgID int64, params ListMembersParams) (response.Paginated[Member], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return response.Paginated[Member]{}, err
	}
	if params.Role != "" && !ValidMemberRole(params.Role) {
		return response.Paginated[Member]{}, &ValidationError{Fields: map[string]string{"role": "Role must be Owner, Administrator, or Baseline Access."}}
	}
	params.OrgID = orgID
	return s.store.ListOrgMembersPage(ctx, params)
}

// GetMember returns one member. An account outside the organization is
// reported as ErrNotFound.
func (s *Service) GetMember(ctx context.Context, p access.Principal, orgID, accountID int64) (Member, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return Member{}, err
	}
	member, found, err := s.store.GetOrgMember(ctx, orgID, accountID)
	if err != nil {
		return Member{}, err
	}
	if !found {
		return Member{}, ErrNotFound
	}
	return member, nil
}

// ListMemberTeams returns a page of the organization teams the member
// belongs to. params.OrgID is set from orgID.
func (s *Service) ListMemberTeams(ctx context.Context, p access.Principal, orgID, accountID int64, params ListTeamsParams) (response.Paginated[Team], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return response.Paginated[Team]{}, err
	}
	if err := s.requireMember(ctx, orgID, accountID); err != nil {
		return response.Paginated[Team]{}, err
	}
	params.OrgID = orgID
	return s.store.ListAccountTeamsPage(ctx, params, accountID)
}

// RemoveMember removes the account from the organization, revokes the access
// it held there, and closes its live sessions. Removing an account that is not
// a member succeeds without effect. The last owner cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, p access.Principal, orgID, accountID int64) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return err
	}
	lastOwner, err := s.isLastOwner(ctx, orgID, accountID)
	if err != nil {
		return err
	}
	if lastOwner {
		return ErrLastOwner
	}

	actorID := p.Subject.ID
	if err := s.store.RemoveOrgMemberAccess(ctx, orgID, accountID, &actorID, MemberRemovalReason); err != nil {
		return err
	}
	s.revokeSessions(ctx, orgID, accountID)
	s.publishCredentialsRevoked(ctx, accountID)
	s.invalidatePrincipals(orgID, accountID)
	s.emit(ctx, p, orgID, ActionMemberRemoved, "account", accountID, nil)
	return nil
}

// UpdateMemberRole replaces the member's builtin organization role with role.
// The caller must be able to transfer ownership, and granting a role with
// owner-level permissions requires the caller to hold each of them. The last
// owner cannot be demoted.
func (s *Service) UpdateMemberRole(ctx context.Context, p access.Principal, orgID, accountID int64, role string) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgTransferOwnership); err != nil {
		return err
	}
	fields := map[string]string{}
	if role == "" {
		addField(fields, "role", "Role is required.")
	}
	if !ValidMemberRole(role) {
		addField(fields, "role", "Role must be Owner, Administrator, or Baseline Access.")
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}

	if role != access.BuiltinOrgOwnerRole {
		lastOwner, err := s.isLastOwner(ctx, orgID, accountID)
		if err != nil {
			return err
		}
		if lastOwner {
			return ErrLastOwner
		}
	}

	roles, err := s.store.ListOrgRoles(ctx, orgID)
	if err != nil {
		return err
	}
	var target database.Role
	var replaced []int64
	for _, r := range roles {
		if !r.IsBuiltin || !ValidMemberRole(r.Name) {
			continue
		}
		replaced = append(replaced, r.ID)
		if r.Name == role && target.ID == 0 {
			target = r
		}
	}
	if target.ID == 0 {
		return ErrNotFound
	}
	for _, permission := range rbac.PrivilegedPermissions(target) {
		if !s.requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
			return ErrForbidden
		}
	}
	if err := s.requireMember(ctx, orgID, accountID); err != nil {
		return err
	}

	if err := s.store.ReplaceOrgMemberBuiltinRole(ctx, orgID, accountID, target.ID, replaced, p.Subject.ID); err != nil {
		return err
	}
	s.invalidateOrgPolicy(orgID)
	s.invalidatePrincipals(orgID, accountID)
	s.emit(ctx, p, orgID, ActionMemberRoleChanged, "account", accountID, map[string]string{"role_id": strconv.FormatInt(target.ID, 10)})
	return nil
}

func (s *Service) requireMember(ctx context.Context, orgID, accountID int64) error {
	ok, err := s.store.IsOrgMember(ctx, orgID, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// isLastOwner reports whether accountID is the organization's only owner.
func (s *Service) isLastOwner(ctx context.Context, orgID, accountID int64) (bool, error) {
	roles, err := s.store.ListOrgRoles(ctx, orgID)
	if err != nil {
		return false, err
	}
	var ownerRoleID int64
	for _, r := range roles {
		if r.Name == access.BuiltinOrgOwnerRole && r.IsBuiltin {
			ownerRoleID = r.ID
			break
		}
	}
	if ownerRoleID == 0 {
		return false, nil
	}
	n, err := s.store.CountRoleBinding(ctx, orgID, ownerRoleID, "org", orgID)
	if err != nil {
		return false, err
	}
	if n > 1 {
		return false, nil
	}
	return s.store.AccountHasRoleBinding(ctx, orgID, ownerRoleID, accountID, "org", orgID)
}

// revokeSessions runs after the membership change committed, so a client
// disconnect must not skip it. The revoker queues its own retry; a failure here
// only means the retry could not be arranged and is logged.
func (s *Service) revokeSessions(ctx context.Context, orgID, accountID int64) {
	if s.revoker == nil {
		return
	}
	if _, err := s.revoker.RevokeOrgAccount(context.WithoutCancel(ctx), strconv.FormatInt(orgID, 10), strconv.FormatInt(accountID, 10)); err != nil {
		s.logger.WarnContext(ctx, "organization member session revocation failed", slog.Int64("org_id", orgID), slog.Int64("target_account_id", accountID), slog.Any("error", err))
	}
}

// publishCredentialsRevoked announces the removed member's revoked org access.
// Delivery is best-effort: a failure is logged and never fails the removal.
func (s *Service) publishCredentialsRevoked(ctx context.Context, accountID int64) {
	if s.inval == nil {
		return
	}
	if err := s.inval.Publish(context.WithoutCancel(ctx), invalidate.TopicCredentials, "account:"+strconv.FormatInt(accountID, 10)); err != nil {
		s.logger.WarnContext(ctx, "credential invalidation publish failed", slog.Any("error", err))
	}
}
