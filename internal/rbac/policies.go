package rbac

import (
	"context"
	"slices"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// PolicyItem is a policy binding as lists and lookups return it.
type PolicyItem = database.WorkspacePolicyListItem

// Binding is a stored role binding.
type Binding = database.RoleBinding

// ListPoliciesParams filters and pages an organization policy list.
type ListPoliciesParams = database.ListOrgPoliciesParams

// ListWorkspacePoliciesParams filters and pages a workspace policy list.
type ListWorkspacePoliciesParams = database.ListWorkspacePoliciesParams

// PrivilegedPermissions returns the owner-level permissions role grants.
// Only an actor who already holds each of them may grant or revoke the role,
// or add them to or remove them from a custom role.
func PrivilegedPermissions(role Role) []string {
	protected := make([]string, 0, 2)
	seen := map[string]bool{}
	add := func(permission string) {
		if !seen[permission] {
			protected = append(protected, permission)
			seen[permission] = true
		}
	}
	for _, permission := range role.Permissions {
		switch permission {
		case access.PermOrgDelete, access.PermOrgTransferOwnership:
			add(permission)
		}
	}
	if isOrgOwnerRole(role) {
		add(access.PermOrgDelete)
		add(access.PermOrgTransferOwnership)
	}
	return protected
}

// ValidSubjectType reports whether subjectType names a subject an
// organization policy can bind.
func ValidSubjectType(subjectType string) bool {
	switch subjectType {
	case access.SubjectTypeAccount, access.SubjectTypeTeam, access.SubjectTypeOrgMembers:
		return true
	default:
		return false
	}
}

// ListPolicies returns a page of the organization-level policy bindings.
// params.OrgID is set from orgID.
func (s *Service) ListPolicies(ctx context.Context, p access.Principal, orgID int64, params ListPoliciesParams) (response.Paginated[PolicyItem], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyRead); err != nil {
		return response.Paginated[PolicyItem]{}, err
	}
	params.OrgID = orgID
	return s.store.ListOrgPoliciesPage(ctx, params)
}

// GetPolicy returns one organization-level binding. Bindings on workspaces and
// the resources inside them are reported as ErrNotFound.
func (s *Service) GetPolicy(ctx context.Context, p access.Principal, orgID, bindingID int64) (PolicyItem, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyRead); err != nil {
		return PolicyItem{}, err
	}
	binding, err := s.loadOrgBinding(ctx, orgID, bindingID)
	if err != nil {
		return PolicyItem{}, err
	}
	return s.store.GetPolicyBindingItem(ctx, orgID, binding)
}

type GrantPolicyInput struct {
	RoleID      int64
	SubjectType string
	SubjectID   int64
}

// GrantPolicy binds an organization role to a subject on the organization.
// The subject must belong to the organization, and granting a role with
// owner-level permissions requires the caller to hold each of them. Granting
// an existing binding again succeeds without effect.
func (s *Service) GrantPolicy(ctx context.Context, p access.Principal, orgID int64, in GrantPolicyInput) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyModify); err != nil {
		return err
	}
	fields := map[string]string{}
	if in.RoleID <= 0 {
		addField(fields, "role_id", "Role is required.")
	}
	if !ValidSubjectType(in.SubjectType) {
		addField(fields, "subject_type", "Subject type must be account, team, or org_members.")
	}
	if in.SubjectID <= 0 {
		addField(fields, "subject_id", "Subject is required.")
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}

	exists, err := s.subjectExists(ctx, orgID, in.SubjectType, in.SubjectID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	role, err := s.loadRole(ctx, orgID, in.RoleID)
	if err != nil {
		return err
	}
	if role.ScopeType != "org" || role.WorkspaceID != nil {
		return &ValidationError{Fields: map[string]string{"role_id": "Role scope must match resource type."}}
	}
	if !s.holdsPrivileged(ctx, p, orgID, role) {
		return &RefusalError{Err: ErrPrivilegedPolicy, Role: role}
	}
	if err := s.store.BindRole(ctx, orgID, role.ID, in.SubjectType, in.SubjectID, "org", orgID, p.Subject.ID); err != nil {
		return err
	}
	s.emit(ctx, p, orgID, ActionPolicyGranted, "role", role.ID, map[string]string{
		"subject_type":  in.SubjectType,
		"subject_id":    strconv.FormatInt(in.SubjectID, 10),
		"resource_type": "org",
		"resource_id":   strconv.FormatInt(orgID, 10),
	})
	return nil
}

// RevokePolicy removes an organization-level binding and returns it. Revoking
// a role with owner-level permissions requires the caller to hold each of
// them, and the organization's last owner binding cannot be revoked.
func (s *Service) RevokePolicy(ctx context.Context, p access.Principal, orgID, bindingID int64) (Binding, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermPolicyModify); err != nil {
		return Binding{}, err
	}
	binding, err := s.loadOrgBinding(ctx, orgID, bindingID)
	if err != nil {
		return Binding{}, err
	}
	role, err := s.loadRole(ctx, orgID, binding.RoleID)
	if err != nil {
		return Binding{}, err
	}
	if !s.holdsPrivileged(ctx, p, orgID, role) {
		return Binding{}, &RefusalError{Err: ErrPrivilegedPolicy, Role: role, Binding: binding}
	}
	if isOrgOwnerRole(role) {
		count, err := s.store.CountRoleBindings(ctx, orgID, role.ID, "org", orgID)
		if err != nil {
			return Binding{}, err
		}
		if count <= 1 {
			return Binding{}, &RefusalError{Err: ErrLastOwner, Role: role, Binding: binding}
		}
	}
	if err := s.store.UnbindRole(ctx, binding.ID, orgID); err != nil {
		return Binding{}, err
	}
	s.emit(ctx, p, orgID, ActionPolicyRevoked, "role_binding", binding.ID, map[string]string{
		"role_id":       strconv.FormatInt(binding.RoleID, 10),
		"subject_type":  binding.SubjectType,
		"subject_id":    strconv.FormatInt(binding.SubjectID, 10),
		"resource_type": binding.ResourceType,
		"resource_id":   strconv.FormatInt(binding.ResourceID, 10),
	})
	return binding, nil
}

// loadOrgBinding returns a binding on the organization itself.
func (s *Service) loadOrgBinding(ctx context.Context, orgID, bindingID int64) (Binding, error) {
	binding, found, err := s.store.GetRoleBinding(ctx, bindingID, orgID)
	if err != nil {
		return Binding{}, err
	}
	if !found || binding.ResourceType != "org" || binding.ResourceID != orgID {
		return Binding{}, ErrNotFound
	}
	return binding, nil
}

func (s *Service) holdsPrivileged(ctx context.Context, p access.Principal, orgID int64, role Role) bool {
	return s.holdsOrgPermissions(ctx, p, orgID, PrivilegedPermissions(role))
}

// holdsPrivilegedChange reports whether the actor holds every owner-level
// permission that replacing role's permissions with permissions would add or
// remove. Owner-level permissions the role keeps are not checked again.
func (s *Service) holdsPrivilegedChange(ctx context.Context, p access.Principal, orgID int64, role Role, permissions []string) bool {
	before := PrivilegedPermissions(role)
	after := PrivilegedPermissions(Role{Permissions: permissions})
	changed := make([]string, 0, len(before)+len(after))
	for _, permission := range before {
		if !slices.Contains(after, permission) {
			changed = append(changed, permission)
		}
	}
	for _, permission := range after {
		if !slices.Contains(before, permission) {
			changed = append(changed, permission)
		}
	}
	return s.holdsOrgPermissions(ctx, p, orgID, changed)
}

func (s *Service) holdsOrgPermissions(ctx context.Context, p access.Principal, orgID int64, permissions []string) bool {
	for _, permission := range permissions {
		if !s.requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
			return false
		}
	}
	return true
}

// subjectExists reports whether the subject belongs to the organization.
func (s *Service) subjectExists(ctx context.Context, orgID int64, subjectType string, subjectID int64) (bool, error) {
	switch subjectType {
	case access.SubjectTypeAccount:
		_, found, err := s.store.GetAccount(ctx, subjectID)
		if err != nil || !found {
			return false, err
		}
		return s.store.IsOrgMember(ctx, orgID, subjectID)
	case access.SubjectTypeTeam:
		team, found, err := s.store.GetTeamByID(ctx, subjectID)
		if err != nil || !found {
			return false, err
		}
		return team.OrgID == orgID, nil
	case access.SubjectTypeOrgMembers:
		return subjectID == orgID, nil
	default:
		return false, nil
	}
}

func isOrgOwnerRole(role Role) bool {
	return role.IsBuiltin && role.Name == access.BuiltinOrgOwnerRole && role.ScopeType == "org" && role.WorkspaceID == nil
}
