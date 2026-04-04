package access

import "errors"

var (
	ErrBuiltinRole            = errors.New("builtin role")
	ErrRoleNotFound           = errors.New("role not found")
	ErrUnknownPermission      = errors.New("unknown permission")
	ErrInvalidScopePermission = errors.New("permission is not valid for scope")
)
