package access

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sqlwarden/internal/audit"
)

// Audited actions emitted by access administration use cases.
const (
	ActionRoleCreated  = "access.role.created"
	ActionRoleUpdated  = "access.role.updated"
	ActionRoleDeleted  = "access.role.deleted"
	ActionPolicyGrant  = "access.policy.granted"
	ActionPolicyRevoke = "access.policy.revoked"

	auditResourceRole    = "role"
	auditResourceBinding = "role_binding"
)

// Service is the application service for role and policy administration. It
// owns the authorization use cases that used to live in the HTTP transport:
// tenant boundaries, role scope matching, protected owner policy, and the
// last-owner guard. Transport layers keep request decoding, read models, and
// status-code mapping.
//
// Every use case enforces the invariants core RBAC depends on: an account
// subject must already be an organization member, a role may only be bound to
// a resource its scope applies to, builtin roles stay immutable, and role
// binding writes are idempotent.
type Service struct {
	store  Store
	roles  RoleManager
	policy PolicyEvaluator
	audit  audit.Writer
}

// RoleManager owns role and binding writes plus cache invalidation.
type RoleManager interface {
	CreateRole(ctx context.Context, orgID int64, workspaceID *int64, name, description, scopeType string, permissions []string) (int64, error)
	UpdateRole(ctx context.Context, roleID, orgID int64, name, description string, permissions []string) error
	DeleteRole(ctx context.Context, roleID, orgID int64) error
	BindRole(ctx context.Context, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID, createdBy int64) error
	UnbindRole(ctx context.Context, bindingID, orgID int64) error
}

// NewService returns the access application service. The policy evaluator is
// the edition-decorated decision path, so an edition restriction also applies
// to the privileged-grant checks made here.
// The writer is the edition-composed audit sink: every administrative write
// here emits its own audit intent, so a transport cannot forget to.
func NewService(store Store, roles RoleManager, policy PolicyEvaluator, writer audit.Writer) *Service {
	if writer == nil {
		writer = audit.Discard
	}
	return &Service{store: store, roles: roles, policy: policy, audit: writer}
}

// OrgRoleInput describes a custom organization role.
type OrgRoleInput struct {
	OrgID int64
	// ActorID is the authenticated account performing the administration. It
	// is carried on the input rather than read from a transport context, so
	// the audit trail names an actor without this package knowing about HTTP.
	ActorID     int64
	Name        string
	Description string
	ScopeType   string
	Permissions []string
}

// CreateOrgRole creates a custom organization-scoped role. Role scope is
// validated before creation so an invalid scope never reaches persistence.
func (s *Service) CreateOrgRole(ctx context.Context, input OrgRoleInput) (int64, error) {
	if input.ScopeType != scopeOrg {
		return 0, fmt.Errorf("%w: organization roles must use scope %q", ErrInvalidRoleScope, scopeOrg)
	}
	if err := validatePermissionScope(input.Permissions, input.ScopeType); err != nil {
		return 0, err
	}
	roleID, err := s.roles.CreateRole(ctx, input.OrgID, nil, input.Name, input.Description, input.ScopeType, input.Permissions)
	if err != nil {
		return 0, err
	}
	event := s.event(ActionRoleCreated, auditResourceRole, roleID, &input.OrgID, actor(input.ActorID))
	event.Metadata = map[string]string{"scope_type": input.ScopeType}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		return 0, err
	}
	return roleID, nil
}

// WorkspaceRoleInput describes a custom workspace-owned role. The scope may be
// the workspace itself or a resource kind inside it.
type WorkspaceRoleInput struct {
	OrgID       int64
	WorkspaceID int64
	ActorID     int64
	Name        string
	Description string
	ScopeType   string
	Permissions []string
}

// CreateWorkspaceRole creates a custom role owned by a workspace.
func (s *Service) CreateWorkspaceRole(ctx context.Context, input WorkspaceRoleInput) (int64, error) {
	if !workspaceRoleScopes[input.ScopeType] {
		return 0, fmt.Errorf("%w: workspace roles cannot use scope %q", ErrInvalidRoleScope, input.ScopeType)
	}
	if err := validatePermissionScope(input.Permissions, input.ScopeType); err != nil {
		return 0, err
	}
	workspaceID := input.WorkspaceID
	roleID, err := s.roles.CreateRole(ctx, input.OrgID, &workspaceID, input.Name, input.Description, input.ScopeType, input.Permissions)
	if err != nil {
		return 0, err
	}
	event := s.event(ActionRoleCreated, auditResourceRole, roleID, &input.OrgID, actor(input.ActorID))
	event.Metadata = map[string]string{
		"scope_type":   input.ScopeType,
		"workspace_id": strconv.FormatInt(workspaceID, 10),
	}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		return 0, err
	}
	return roleID, nil
}

// UpdateOrgRoleInput replaces the definition of a custom organization role.
type UpdateOrgRoleInput struct {
	OrgID       int64
	RoleID      int64
	ActorID     int64
	Name        string
	Description string
	Permissions []string
}

// UpdateOrgRole replaces the name, description, and permission set of a custom
// organization role. Builtin roles are rejected by the enforcer.
func (s *Service) UpdateOrgRole(ctx context.Context, input UpdateOrgRoleInput) error {
	role, found, err := s.store.Role(ctx, input.OrgID, input.RoleID)
	if err != nil {
		return err
	}
	if !found || role.ScopeType != scopeOrg || role.WorkspaceID != nil {
		return ErrRoleNotFound
	}
	if err := s.roles.UpdateRole(ctx, input.RoleID, input.OrgID, input.Name, input.Description, input.Permissions); err != nil {
		return err
	}
	return audit.Emit(ctx, s.audit, s.event(ActionRoleUpdated, auditResourceRole, input.RoleID, &input.OrgID, actor(input.ActorID)))
}

// UpdateWorkspaceRoleInput replaces the definition of a custom workspace role.
type UpdateWorkspaceRoleInput struct {
	OrgID       int64
	WorkspaceID int64
	RoleID      int64
	ActorID     int64
	Name        string
	Description string
	Permissions []string
}

// UpdateWorkspaceRole updates a custom role after confirming it belongs to the
// workspace in the request path.
func (s *Service) UpdateWorkspaceRole(ctx context.Context, input UpdateWorkspaceRoleInput) error {
	if _, err := s.workspaceRole(ctx, input.OrgID, input.WorkspaceID, input.RoleID); err != nil {
		return err
	}
	if err := s.roles.UpdateRole(ctx, input.RoleID, input.OrgID, input.Name, input.Description, input.Permissions); err != nil {
		return err
	}
	event := s.event(ActionRoleUpdated, auditResourceRole, input.RoleID, &input.OrgID, actor(input.ActorID))
	event.Metadata = map[string]string{"workspace_id": strconv.FormatInt(input.WorkspaceID, 10)}
	return audit.Emit(ctx, s.audit, event)
}

// DeleteOrgRoleInput removes a custom organization role.
type DeleteOrgRoleInput struct {
	OrgID   int64
	RoleID  int64
	ActorID int64
}

// DeleteOrgRole deletes an unbound custom organization role.
func (s *Service) DeleteOrgRole(ctx context.Context, input DeleteOrgRoleInput) error {
	role, found, err := s.store.Role(ctx, input.OrgID, input.RoleID)
	if err != nil {
		return err
	}
	if !found || role.ScopeType != scopeOrg || role.WorkspaceID != nil {
		return ErrRoleNotFound
	}
	if err := s.roles.DeleteRole(ctx, input.RoleID, input.OrgID); err != nil {
		return err
	}
	return audit.Emit(ctx, s.audit, s.event(ActionRoleDeleted, auditResourceRole, input.RoleID, &input.OrgID, actor(input.ActorID)))
}

// DeleteWorkspaceRoleInput removes a custom workspace-owned role.
type DeleteWorkspaceRoleInput struct {
	OrgID       int64
	WorkspaceID int64
	RoleID      int64
	ActorID     int64
}

// DeleteWorkspaceRole deletes an unbound custom role owned by the workspace.
func (s *Service) DeleteWorkspaceRole(ctx context.Context, input DeleteWorkspaceRoleInput) error {
	if _, err := s.workspaceRole(ctx, input.OrgID, input.WorkspaceID, input.RoleID); err != nil {
		return err
	}
	if err := s.roles.DeleteRole(ctx, input.RoleID, input.OrgID); err != nil {
		return err
	}
	event := s.event(ActionRoleDeleted, auditResourceRole, input.RoleID, &input.OrgID, actor(input.ActorID))
	event.Metadata = map[string]string{"workspace_id": strconv.FormatInt(input.WorkspaceID, 10)}
	return audit.Emit(ctx, s.audit, event)
}

// GrantOrgPolicyInput binds an organization role to a subject at the org.
type GrantOrgPolicyInput struct {
	OrgID       int64
	GrantorID   int64
	RoleID      int64
	SubjectType string
	SubjectID   int64
}

// GrantOrgPolicy binds an organization-scoped role to a subject of the same
// organization. The write is idempotent: repeating a grant is a no-op.
func (s *Service) GrantOrgPolicy(ctx context.Context, input GrantOrgPolicyInput) error {
	if err := s.requireOrgSubject(ctx, input.OrgID, input.SubjectType, input.SubjectID); err != nil {
		return err
	}

	role, found, err := s.store.Role(ctx, input.OrgID, input.RoleID)
	if err != nil {
		return err
	}
	if !found {
		return ErrRoleNotFound
	}
	if role.ScopeType != scopeOrg || role.WorkspaceID != nil {
		return ErrRoleScopeMismatch
	}
	if err := s.requireProtectedOrgPolicyAuthority(ctx, input.OrgID, input.GrantorID, role); err != nil {
		return err
	}

	if err := s.roles.BindRole(ctx, input.OrgID, input.RoleID, input.SubjectType, input.SubjectID, resourceOrg, input.OrgID, input.GrantorID); err != nil {
		return err
	}
	event := s.event(ActionPolicyGrant, resourceOrg, input.OrgID, &input.OrgID, actor(input.GrantorID))
	event.Metadata = map[string]string{
		"role_id":      strconv.FormatInt(input.RoleID, 10),
		"subject_type": input.SubjectType,
		"subject_id":   strconv.FormatInt(input.SubjectID, 10),
	}
	return audit.Emit(ctx, s.audit, event)
}

// RevokeOrgPolicyInput removes one organization policy binding.
type RevokeOrgPolicyInput struct {
	OrgID     int64
	GrantorID int64
	BindingID int64
}

// RevokeOrgPolicy removes an organization policy binding. It refuses to remove
// the last owner binding, which would leave the organization unadministrable.
func (s *Service) RevokeOrgPolicy(ctx context.Context, input RevokeOrgPolicyInput) (RoleBinding, error) {
	binding, err := s.OrgPolicyBinding(ctx, input.OrgID, input.BindingID)
	if err != nil {
		return RoleBinding{}, err
	}

	role, found, err := s.store.Role(ctx, input.OrgID, binding.RoleID)
	if err != nil {
		return RoleBinding{}, err
	}
	if !found {
		return RoleBinding{}, ErrRoleNotFound
	}
	if err := s.requireProtectedOrgPolicyAuthority(ctx, input.OrgID, input.GrantorID, role); err != nil {
		return RoleBinding{}, err
	}

	if isOrgOwnerRole(role) {
		count, err := s.store.CountRoleBindings(ctx, input.OrgID, binding.RoleID, resourceOrg, input.OrgID)
		if err != nil {
			return RoleBinding{}, err
		}
		if count <= 1 {
			return RoleBinding{}, ErrLastOwnerPolicy
		}
	}

	if err := s.roles.UnbindRole(ctx, input.BindingID, input.OrgID); err != nil {
		return RoleBinding{}, err
	}
	event := s.event(ActionPolicyRevoke, auditResourceBinding, input.BindingID, &input.OrgID, actor(input.GrantorID))
	event.Metadata = map[string]string{"role_id": strconv.FormatInt(binding.RoleID, 10)}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		return RoleBinding{}, err
	}
	return binding, nil
}

// OrgPolicyBinding returns an organization-level binding after confirming it
// belongs to the organization in the request path.
func (s *Service) OrgPolicyBinding(ctx context.Context, orgID, bindingID int64) (RoleBinding, error) {
	binding, found, err := s.store.RoleBinding(ctx, orgID, bindingID)
	if err != nil {
		return RoleBinding{}, err
	}
	if !found || binding.ResourceType != resourceOrg || binding.ResourceID != orgID {
		return RoleBinding{}, ErrRoleBindingNotFound
	}
	return binding, nil
}

// GrantWorkspacePolicyInput binds a role to the workspace or to a resource
// inside it.
type GrantWorkspacePolicyInput struct {
	OrgID        int64
	WorkspaceID  int64
	GrantorID    int64
	RoleID       int64
	SubjectType  string
	SubjectID    int64
	ResourceType string
	ResourceID   int64
}

// GrantWorkspacePolicy binds a role to a resource that must belong to the
// workspace in the request path, so a policy write cannot reach another
// workspace or another organization.
func (s *Service) GrantWorkspacePolicy(ctx context.Context, input GrantWorkspacePolicyInput) (int64, error) {
	if input.SubjectType == SubjectTypeWorkspaceMembers {
		if input.SubjectID != input.WorkspaceID {
			return 0, ErrSubjectNotFound
		}
	} else if err := s.requireOrgSubject(ctx, input.OrgID, input.SubjectType, input.SubjectID); err != nil {
		return 0, err
	}

	resourceID, err := s.resolveWorkspaceResource(ctx, input.WorkspaceID, input.ResourceType, input.ResourceID)
	if err != nil {
		return 0, err
	}

	role, found, err := s.store.Role(ctx, input.OrgID, input.RoleID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, ErrRoleNotFound
	}
	if role.ScopeType != input.ResourceType {
		return 0, ErrRoleScopeMismatch
	}
	if role.WorkspaceID != nil && *role.WorkspaceID != input.WorkspaceID {
		return 0, ErrRoleNotFound
	}

	err = s.roles.BindRole(ctx, input.OrgID, input.RoleID, input.SubjectType, input.SubjectID, input.ResourceType, resourceID, input.GrantorID)
	if err != nil {
		return 0, err
	}
	event := s.event(ActionPolicyGrant, input.ResourceType, resourceID, &input.OrgID, actor(input.GrantorID))
	event.Metadata = map[string]string{
		"role_id":      strconv.FormatInt(input.RoleID, 10),
		"subject_type": input.SubjectType,
		"subject_id":   strconv.FormatInt(input.SubjectID, 10),
		"workspace_id": strconv.FormatInt(input.WorkspaceID, 10),
	}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		return 0, err
	}
	return resourceID, nil
}

// RevokeWorkspacePolicyInput removes one workspace-scoped policy binding.
type RevokeWorkspacePolicyInput struct {
	OrgID       int64
	WorkspaceID int64
	BindingID   int64
	ActorID     int64
}

// RevokeWorkspacePolicy removes a binding whose resource belongs to the
// workspace in the request path.
func (s *Service) RevokeWorkspacePolicy(ctx context.Context, input RevokeWorkspacePolicyInput) (RoleBinding, error) {
	orgID, workspaceID, bindingID := input.OrgID, input.WorkspaceID, input.BindingID
	binding, err := s.WorkspacePolicyBinding(ctx, orgID, workspaceID, bindingID)
	if err != nil {
		return RoleBinding{}, err
	}
	if err := s.roles.UnbindRole(ctx, bindingID, orgID); err != nil {
		return RoleBinding{}, err
	}
	event := s.event(ActionPolicyRevoke, auditResourceBinding, bindingID, &orgID, actor(input.ActorID))
	event.Metadata = map[string]string{
		"role_id":      strconv.FormatInt(binding.RoleID, 10),
		"workspace_id": strconv.FormatInt(workspaceID, 10),
	}
	if err := audit.Emit(ctx, s.audit, event); err != nil {
		return RoleBinding{}, err
	}
	return binding, nil
}

// WorkspacePolicyBinding returns a binding after confirming its resource
// belongs to the workspace in the request path.
func (s *Service) WorkspacePolicyBinding(ctx context.Context, orgID, workspaceID, bindingID int64) (RoleBinding, error) {
	binding, found, err := s.store.RoleBinding(ctx, orgID, bindingID)
	if err != nil {
		return RoleBinding{}, err
	}
	if !found {
		return RoleBinding{}, ErrRoleBindingNotFound
	}
	if _, err := s.resolveWorkspaceResource(ctx, workspaceID, binding.ResourceType, binding.ResourceID); err != nil {
		return RoleBinding{}, ErrRoleBindingNotFound
	}
	return binding, nil
}

// EffectivePermissionsInput asks for one account's permissions on one resource.
// A zero ResourceID is only meaningful for the organization itself.
type EffectivePermissionsInput struct {
	AccountID    int64
	OrgID        int64
	ResourceType string
	ResourceID   int64
}

// EffectivePermissions resolves the permissions an account holds on a resource
// after confirming the resource belongs to the organization in the request
// path. Resolution runs through the edition-decorated evaluator, so an edition
// restriction is reflected in what the UI is told it may do.
func (s *Service) EffectivePermissions(ctx context.Context, input EffectivePermissionsInput) (int64, []string, error) {
	member, err := s.store.IsOrgMember(ctx, input.OrgID, input.AccountID)
	if err != nil {
		return 0, nil, err
	}
	if !member {
		return 0, nil, ErrSubjectNotFound
	}
	resourceID, err := s.resolveOrgResource(ctx, input.OrgID, input.ResourceType, input.ResourceID)
	if err != nil {
		return 0, nil, err
	}
	permissions, err := s.policy.EffectivePermissions(ctx, input.AccountID, input.OrgID, resourceOrg, input.ResourceType, resourceID)
	if err != nil {
		return 0, nil, err
	}
	return resourceID, permissions, nil
}

// event builds an audit event for an administrative write. Audit emission
// errors reach the caller: an authorization change that could not be audited
// is not reported as applied.
func (s *Service) event(action, resource string, resourceID int64, orgID, actorID *int64) audit.Event {
	return audit.Event{
		OrgID:      orgID,
		AccountID:  actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: strconv.FormatInt(resourceID, 10),
		Outcome:    audit.OutcomeSuccess,
	}
}

// actor turns an administration input's account identifier into an audit
// subject. Zero means no authenticated account was supplied, which is recorded
// as an absent subject rather than as account zero.
func actor(accountID int64) *int64 {
	if accountID == 0 {
		return nil
	}
	return &accountID
}

// requireOrgSubject enforces the org-membership-first gate: a policy subject
// must already belong to the organization, so a binding can never grant
// organization access to an outsider.
func (s *Service) requireOrgSubject(ctx context.Context, orgID int64, subjectType string, subjectID int64) error {
	switch subjectType {
	case SubjectTypeAccount:
		exists, err := s.store.AccountExists(ctx, subjectID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrSubjectNotFound
		}
		member, err := s.store.IsOrgMember(ctx, orgID, subjectID)
		if err != nil {
			return err
		}
		if !member {
			return ErrSubjectNotFound
		}
		return nil
	case SubjectTypeTeam:
		teamOrgID, found, err := s.store.TeamOrg(ctx, subjectID)
		if err != nil {
			return err
		}
		if !found || teamOrgID != orgID {
			return ErrSubjectNotFound
		}
		return nil
	case SubjectTypeOrgMembers:
		if subjectID != orgID {
			return ErrSubjectNotFound
		}
		return nil
	default:
		return ErrInvalidSubjectType
	}
}

// resolveWorkspaceResource returns the resource ID to bind against after
// confirming the resource belongs to workspaceID.
func (s *Service) resolveWorkspaceResource(ctx context.Context, workspaceID int64, resourceType string, resourceID int64) (int64, error) {
	switch resourceType {
	case resourceWorkspace:
		if resourceID != 0 && resourceID != workspaceID {
			return 0, ErrResourceNotFound
		}
		return workspaceID, nil
	case resourceEnvironment:
		owner, found, err := s.store.EnvironmentWorkspace(ctx, resourceID)
		if err != nil {
			return 0, err
		}
		if !found || owner != workspaceID {
			return 0, ErrResourceNotFound
		}
		return resourceID, nil
	case resourceConnection:
		owner, found, err := s.store.ConnectionWorkspace(ctx, resourceID)
		if err != nil {
			return 0, err
		}
		if !found || owner != workspaceID {
			return 0, ErrResourceNotFound
		}
		return resourceID, nil
	default:
		return 0, ErrInvalidResourceType
	}
}

// resolveOrgResource confirms a resource belongs to orgID, walking workspace
// ownership for resources nested inside a workspace.
func (s *Service) resolveOrgResource(ctx context.Context, orgID int64, resourceType string, resourceID int64) (int64, error) {
	switch resourceType {
	case resourceOrg:
		if resourceID == 0 {
			return orgID, nil
		}
		if resourceID != orgID {
			return 0, ErrResourceNotFound
		}
		return orgID, nil
	case resourceWorkspace:
		if err := s.requireWorkspaceInOrg(ctx, orgID, resourceID); err != nil {
			return 0, err
		}
		return resourceID, nil
	case resourceEnvironment, resourceConnection:
		if resourceID == 0 {
			return 0, ErrResourceNotFound
		}
		var (
			workspaceID int64
			found       bool
			err         error
		)
		if resourceType == resourceEnvironment {
			workspaceID, found, err = s.store.EnvironmentWorkspace(ctx, resourceID)
		} else {
			workspaceID, found, err = s.store.ConnectionWorkspace(ctx, resourceID)
		}
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, ErrResourceNotFound
		}
		if err := s.requireWorkspaceInOrg(ctx, orgID, workspaceID); err != nil {
			return 0, err
		}
		return resourceID, nil
	default:
		return 0, ErrInvalidResourceType
	}
}

func (s *Service) requireWorkspaceInOrg(ctx context.Context, orgID, workspaceID int64) error {
	if workspaceID == 0 {
		return ErrResourceNotFound
	}
	owner, found, err := s.store.WorkspaceOrg(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found || owner != orgID {
		return ErrResourceNotFound
	}
	return nil
}

func (s *Service) workspaceRole(ctx context.Context, orgID, workspaceID, roleID int64) (Role, error) {
	role, found, err := s.store.Role(ctx, orgID, roleID)
	if err != nil {
		return Role{}, err
	}
	if !found || role.WorkspaceID == nil || *role.WorkspaceID != workspaceID {
		return Role{}, ErrRoleNotFound
	}
	return role, nil
}

// requireProtectedOrgPolicyAuthority enforces that owner-level policy may only
// be managed by an actor who already holds the permission being handed out.
func (s *Service) requireProtectedOrgPolicyAuthority(ctx context.Context, orgID, grantorID int64, role Role) error {
	for _, permission := range protectedOrgPolicyPermissions(role) {
		if !s.policy.Can(ctx, grantorID, orgID, resourceOrg, resourceOrg, orgID, permission) {
			return ErrProtectedPolicy
		}
	}
	return nil
}

// protectedOrgPolicyPermissions returns the owner-level permissions a role
// hands out. The builtin owner role is treated as granting both, because its
// permission set is not editable and must stay privileged.
func protectedOrgPolicyPermissions(role Role) []string {
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
		case PermOrgDelete, PermOrgTransferOwnership:
			add(permission)
		}
	}
	if isOrgOwnerRole(role) {
		add(PermOrgDelete)
		add(PermOrgTransferOwnership)
	}
	return protected
}

func isOrgOwnerRole(role Role) bool {
	return role.IsBuiltin && role.Name == BuiltinOrgOwnerRole && role.ScopeType == scopeOrg && role.WorkspaceID == nil
}

func validatePermissionScope(permissions []string, scopeType string) error {
	for _, permission := range permissions {
		if !ValidForScope(permission, scopeType) {
			return fmt.Errorf("%w: permission %q is not valid for scope %q", ErrInvalidScopePermission, permission, scopeType)
		}
	}
	return nil
}

const (
	scopeOrg = "org"

	resourceOrg         = "org"
	resourceWorkspace   = "workspace"
	resourceEnvironment = "environment"
	resourceConnection  = "connection"
)

var workspaceRoleScopes = map[string]bool{
	resourceWorkspace:   true,
	resourceEnvironment: true,
	resourceConnection:  true,
}
