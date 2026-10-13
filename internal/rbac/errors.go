package rbac

import (
	"errors"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/uptrace/bun/driver/pgdriver"
)

var (
	ErrForbidden  = access.ErrForbidden
	ErrNotFound   = access.ErrNotFound
	ErrValidation = errors.New("rbac: validation failed")
	// ErrBuiltinRole reports an attempt to change or delete a builtin role.
	ErrBuiltinRole = access.ErrBuiltinRole
	// ErrRoleInUse reports a role that policy bindings still reference. The
	// error is an access.RoleInUseError carrying the binding count.
	ErrRoleInUse = access.ErrRoleInUse
	// ErrPrivilegedPolicy reports a grant or revoke of a role with owner-level
	// permissions the caller does not hold, or a role update that adds or
	// removes such a permission.
	ErrPrivilegedPolicy = errors.New("rbac: privileged policy requires the caller to hold its permissions")
	// ErrLastOwner reports a revoke that would leave the organization without
	// an owner binding.
	ErrLastOwner = errors.New("rbac: organization must keep an owner")
)

// ValidationError carries field validation messages. The only caller-supplied
// values they repeat are submitted permission names.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// RefusalError reports a policy change an RBAC invariant refused. Err is
// ErrPrivilegedPolicy or ErrLastOwner; Role and Binding identify the refused
// change for logging.
type RefusalError struct {
	Err     error
	Role    database.Role
	Binding database.RoleBinding
}

func (e *RefusalError) Error() string { return e.Err.Error() }
func (e *RefusalError) Unwrap() error { return e.Err }

func isUniqueViolation(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
