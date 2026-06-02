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
