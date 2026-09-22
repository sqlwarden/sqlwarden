package access

import "errors"

var (
	ErrBuiltinRole            = errors.New("builtin role")
	ErrRoleNotFound           = errors.New("role not found")
	ErrRoleInUse              = errors.New("role has policy bindings")
	ErrUnknownPermission      = errors.New("unknown permission")
	ErrInvalidScopePermission = errors.New("permission is not valid for scope")
)

type RoleInUseError struct {
	BindingCount int
}

func (e RoleInUseError) Error() string {
	return ErrRoleInUse.Error()
}

func (e RoleInUseError) Unwrap() error {
	return ErrRoleInUse
}

// Application service errors. Transport layers map these to status codes.
var (
	// ErrRoleBindingNotFound reports a binding that does not exist or does not
	// belong to the organization, workspace, or resource in the request path.
	ErrRoleBindingNotFound = errors.New("role binding not found")
	// ErrSubjectNotFound reports a policy subject that does not exist or is not
	// part of the organization. Org membership is the first access gate.
	ErrSubjectNotFound = errors.New("policy subject not found")
	// ErrResourceNotFound reports a target resource outside the requested
	// organization or workspace.
	ErrResourceNotFound = errors.New("policy resource not found")
	// ErrInvalidSubjectType reports an unsupported policy subject type.
	ErrInvalidSubjectType = errors.New("invalid policy subject type")
	// ErrInvalidResourceType reports an unsupported policy resource type.
	ErrInvalidResourceType = errors.New("invalid policy resource type")
	// ErrInvalidRoleScope reports a role scope that is not allowed for the
	// owner the role is being created under.
	ErrInvalidRoleScope = errors.New("invalid role scope")
	// ErrRoleScopeMismatch reports a role whose scope does not match the
	// resource it would be bound to.
	ErrRoleScopeMismatch = errors.New("role scope does not match resource")
	// ErrProtectedPolicy reports an attempt to manage owner-level policy
	// without already holding the permission it grants.
	ErrProtectedPolicy = errors.New("protected organization policy")
	// ErrLastOwnerPolicy reports an attempt to revoke the last owner binding of
	// an organization.
	ErrLastOwnerPolicy = errors.New("last organization owner policy")
)
