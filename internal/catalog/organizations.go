package catalog

import (
	"context"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// MaxOrgSlugLength bounds an organization slug so it stays usable in a URL
// path segment.
const MaxOrgSlugLength = 64

// Organizations lists every organization for instance administration.
func (s *Service) Organizations(ctx context.Context, params database.ListOrganizationsParams) (response.Paginated[database.OrganizationListItem], error) {
	return s.store.Organizations(ctx, params)
}

// AccountOrganizations lists the organizations visible to an account.
func (s *Service) AccountOrganizations(ctx context.Context, params database.ListAccountOrgsParams) (response.Paginated[database.AccountOrganizationListItem], error) {
	return s.store.AccountOrganizations(ctx, params)
}

// OrganizationMembers lists members of one organization.
func (s *Service) OrganizationMembers(ctx context.Context, params database.ListOrgMembersParams) (response.Paginated[database.OrgMemberListItem], error) {
	return s.store.OrganizationMembers(ctx, params)
}

// OrganizationMember returns one member without exposing persistence details
// to the transport.
func (s *Service) OrganizationMember(ctx context.Context, orgID, accountID int64) (database.OrgMemberListItem, error) {
	member, found, err := s.store.OrganizationMember(ctx, orgID, accountID)
	if err != nil {
		return database.OrgMemberListItem{}, err
	}
	if !found {
		return database.OrgMemberListItem{}, ErrNotFound
	}
	return member, nil
}

// CreateOrganizationInput is a request to create an organization owned by
// OwnerAccountID. An empty Slug is derived from Name.
type CreateOrganizationInput struct {
	Name           string
	Slug           string
	OwnerAccountID int64
}

// CreateOrganization creates an organization, records its creator as the first
// member, and seeds its builtin roles and owner policy in the same
// transaction, so an organization never exists without an administrator.
func (s *Service) CreateOrganization(ctx context.Context, input CreateOrganizationInput) (database.Organization, error) {
	name := trimmed(input.Name)
	slug := ResolveOrgSlug(input)
	var v validator.Validator
	ValidateCreateOrganization(input, &v)
	if err := newValidationError(v); err != nil {
		return database.Organization{}, err
	}

	actor := Actor{AccountID: input.OwnerAccountID}
	org, err := s.store.CreateOrganization(ctx, slug, name, input.OwnerAccountID)
	if err != nil {
		if database.IsUniqueViolation(err) {
			if trimmed(input.Slug) != "" {
				return database.Organization{}, ErrSlugTaken
			}
			return database.Organization{}, ErrNameTaken
		}
		return database.Organization{}, err
	}

	s.grants.InvalidateOrgPolicy(org.ID)
	actor.OrgID = org.ID
	if err := s.auditSuccess(ctx, actor, ActionOrgCreated, resourceOrganization, org.ID, map[string]string{"slug": org.Slug}); err != nil {
		return database.Organization{}, err
	}
	return org, nil
}

// ResolveOrgSlug returns the slug an organization will be created with: the
// requested slug when given, otherwise one derived from the name.
func ResolveOrgSlug(input CreateOrganizationInput) string {
	slug := trimmed(input.Slug)
	if slug == "" {
		slug = Slugify(trimmed(input.Name))
	}
	return slug
}

// ValidateCreateOrganization records the rules an organization must satisfy to
// be created, so a caller can report every problem with a request at once.
func ValidateCreateOrganization(input CreateOrganizationInput, v *validator.Validator) {
	v.CheckField(trimmed(input.Name) != "", "name", "Name is required.")

	slug := ResolveOrgSlug(input)
	v.CheckField(slug != "", "slug", "Slug is required.")
	if slug == "" {
		return
	}
	v.CheckField(IsValidSlug(slug), "slug", "Slug may only contain lowercase letters, numbers, and hyphens.")
	v.CheckField(len(slug) <= MaxOrgSlugLength, "slug", "Slug must be 64 characters or fewer.")
}

// ValidateUpdateOrganization records the rules an organization settings change
// must satisfy.
func ValidateUpdateOrganization(input UpdateOrganizationInput, v *validator.Validator) {
	if input.Name != nil {
		v.CheckField(trimmed(*input.Name) != "", "name", "Name must not be empty.")
	}
	v.CheckField(
		input.Name != nil || input.SchemaSnapshotsEnabled != nil || input.MaskConnectionCredentialsOnEdit != nil,
		"request", "At least one setting is required.")
}

// UpdateOrganizationInput carries the organization settings a request asked to
// change. A nil field is left as it is; at least one must be set.
type UpdateOrganizationInput struct {
	Name                            *string
	SchemaSnapshotsEnabled          *bool
	MaskConnectionCredentialsOnEdit *bool
}

// UpdateOrganizationResult is the updated organization plus the follow-up work
// the change implies.
type UpdateOrganizationResult struct {
	Organization database.Organization
	// SnapshotsDisabled reports that schema snapshots went from enabled to
	// disabled, so the caller must stop snapshot work already scheduled for
	// this organization's connections.
	SnapshotsDisabled bool
}

// UpdateOrganization applies organization settings and reads the stored row
// back, so the caller returns what was persisted rather than what was
// requested.
func (s *Service) UpdateOrganization(ctx context.Context, actor Actor, org database.Organization, input UpdateOrganizationInput) (UpdateOrganizationResult, error) {
	var v validator.Validator
	ValidateUpdateOrganization(input, &v)
	if err := newValidationError(v); err != nil {
		return UpdateOrganizationResult{}, err
	}
	name := input.Name
	if name != nil {
		trimmedName := trimmed(*name)
		name = &trimmedName
	}

	if err := s.store.UpdateOrganizationSettings(ctx, org.ID, name, input.SchemaSnapshotsEnabled, input.MaskConnectionCredentialsOnEdit); err != nil {
		return UpdateOrganizationResult{}, err
	}

	updated, found, err := s.store.Organization(ctx, org.ID)
	if err != nil {
		return UpdateOrganizationResult{}, err
	}
	if !found {
		return UpdateOrganizationResult{}, ErrNotFound
	}

	result := UpdateOrganizationResult{
		Organization:      updated,
		SnapshotsDisabled: org.SchemaSnapshotsEnabled && input.SchemaSnapshotsEnabled != nil && !*input.SchemaSnapshotsEnabled,
	}
	metadata := map[string]string{
		"name_changed":             strconv.FormatBool(name != nil),
		"schema_snapshots_enabled": strconv.FormatBool(updated.SchemaSnapshotsEnabled),
		"mask_credentials_on_edit": strconv.FormatBool(updated.MaskConnectionCredentialsOnEdit),
	}
	if err := s.auditSuccess(ctx, actor, ActionOrgUpdated, resourceOrganization, org.ID, metadata); err != nil {
		return UpdateOrganizationResult{}, err
	}
	return result, nil
}

// DeleteOrganization removes an organization and drops the authorization
// policy cached for it.
func (s *Service) DeleteOrganization(ctx context.Context, actor Actor, org database.Organization) error {
	if err := s.store.DeleteOrganization(ctx, org.ID); err != nil {
		return err
	}
	s.grants.InvalidateOrgPolicy(org.ID)
	return s.auditSuccess(ctx, actor, ActionOrgDeleted, resourceOrganization, org.ID, map[string]string{"slug": org.Slug})
}

// RemoveOrgMember revokes an account's membership and every access that
// depended on it, then drops the live target-database sessions the account
// held in this organization. The last owner cannot be removed, because an
// organization without an owner cannot be administered again.
func (s *Service) RemoveOrgMember(ctx context.Context, actor Actor, orgID, accountID int64) error {
	lastOwner, err := s.isLastOrgOwner(ctx, orgID, accountID)
	if err != nil {
		return err
	}
	if lastOwner {
		return s.auditDenied(ctx, actor, ActionOrgMemberRemoved, resourceOrganization, orgID,
			map[string]string{"target_account_id": strconv.FormatInt(accountID, 10), "reason": "last_owner"}, ErrLastOwner)
	}

	if err := s.store.RemoveOrgMemberAccess(ctx, orgID, accountID, &actor.AccountID, "org_membership_removed"); err != nil {
		return err
	}
	s.sessions.RemoveForOrgAccount(strconv.FormatInt(orgID, 10), strconv.FormatInt(accountID, 10))
	s.grants.InvalidatePrincipals(orgID, accountID)

	return s.auditSuccess(ctx, actor, ActionOrgMemberRemoved, resourceOrganization, orgID,
		map[string]string{"target_account_id": strconv.FormatInt(accountID, 10)})
}

// SetOrgMemberRole replaces a member's builtin organization role. The last
// owner cannot be demoted, and a role name outside the builtin set is
// refused before anything is written.
func (s *Service) SetOrgMemberRole(ctx context.Context, actor Actor, orgID, accountID int64, roleName string) error {
	roleName = trimmed(roleName)
	if !IsBuiltinOrgRole(roleName) {
		return ErrInvalidRole
	}

	metadata := map[string]string{
		"target_account_id": strconv.FormatInt(accountID, 10),
		"requested_role":    roleName,
	}
	if roleName != access.BuiltinOrgOwnerRole {
		lastOwner, err := s.isLastOrgOwner(ctx, orgID, accountID)
		if err != nil {
			return err
		}
		if lastOwner {
			metadata["reason"] = "last_owner"
			return s.auditDenied(ctx, actor, ActionOrgMemberRoleUpdated, resourceOrganization, orgID, metadata, ErrLastOwner)
		}
	}

	roles, err := s.store.OrgRoles(ctx, orgID)
	if err != nil {
		return err
	}
	var roleID int64
	var builtinRoleIDs []int64
	for _, role := range roles {
		if !role.IsBuiltin {
			continue
		}
		if role.Name == roleName {
			roleID = role.ID
		}
		if IsBuiltinOrgRole(role.Name) {
			builtinRoleIDs = append(builtinRoleIDs, role.ID)
		}
	}
	if roleID == 0 {
		return ErrNotFound
	}

	isMember, err := s.store.IsOrgMember(ctx, orgID, accountID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotFound
	}

	if err := s.store.ReplaceOrgMemberBuiltinRole(ctx, orgID, accountID, roleID, builtinRoleIDs, actor.AccountID); err != nil {
		return err
	}
	s.grants.InvalidateOrgPolicy(orgID)
	s.grants.InvalidatePrincipals(orgID, accountID)

	metadata["role_id"] = strconv.FormatInt(roleID, 10)
	return s.auditSuccess(ctx, actor, ActionOrgMemberRoleUpdated, resourceOrganization, orgID, metadata)
}

// isLastOrgOwner reports whether accountID holds the owner role and is the
// only account that does.
func (s *Service) isLastOrgOwner(ctx context.Context, orgID, accountID int64) (bool, error) {
	roles, err := s.store.OrgRoles(ctx, orgID)
	if err != nil {
		return false, err
	}
	var ownerRoleID int64
	for _, role := range roles {
		if role.IsBuiltin && role.Name == access.BuiltinOrgOwnerRole {
			ownerRoleID = role.ID
			break
		}
	}
	if ownerRoleID == 0 {
		return false, nil
	}
	owners, err := s.store.CountRoleBinding(ctx, orgID, ownerRoleID, "org", orgID)
	if err != nil {
		return false, err
	}
	if owners > 1 {
		return false, nil
	}
	return s.store.AccountHasRoleBinding(ctx, orgID, ownerRoleID, accountID, "org", orgID)
}

// IsBuiltinOrgRole reports whether name is one of the builtin organization
// roles a member may be assigned directly.
func IsBuiltinOrgRole(name string) bool {
	switch name {
	case access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, access.BuiltinOrgMemberRole:
		return true
	default:
		return false
	}
}

// Slugify derives a URL-safe organization slug from a display name.
func Slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	s = strings.Trim(s, "-")
	if len(s) > MaxOrgSlugLength {
		s = s[:MaxOrgSlugLength]
	}
	return s
}

// IsValidSlug reports whether s contains only lowercase letters, digits, and
// hyphens, and is not empty.
func IsValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}
