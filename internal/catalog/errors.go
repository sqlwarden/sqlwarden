package catalog

import (
	"errors"

	"github.com/sqlwarden/internal/execution"
)

// Catalog application errors. Transports map these to status codes and
// user-facing messages. None of them carry DSNs, credentials, or row data.
var (
	// ErrNotFound reports a resource that does not exist, or that exists
	// outside the organization or workspace the caller addressed. The two are
	// deliberately indistinguishable so a caller cannot probe another tenant's
	// resource identifiers.
	ErrNotFound = errors.New("catalog resource not found")
	// ErrNameRequired reports a missing or blank resource name.
	ErrNameRequired = errors.New("name is required")
	// ErrNameTaken reports a name that collides with a sibling resource.
	ErrNameTaken = errors.New("name already in use")
	// ErrSlugTaken reports an organization slug that is already registered.
	ErrSlugTaken = errors.New("slug already in use")
	// ErrLastOwner reports removing or demoting the only owner of an
	// organization, which would leave it unadministrable.
	ErrLastOwner = errors.New("cannot remove the last owner of an organization")
	// ErrInvalidRole reports a builtin organization role name outside the
	// owner, administrator, and baseline set.
	ErrInvalidRole = errors.New("invalid organization role")

	// ErrEnvironmentHasConnections reports deleting an environment that still
	// has connections tagged to it.
	ErrEnvironmentHasConnections = errors.New("environment has connections")

	// ErrActiveSessions reports a change that would invalidate live target
	// sessions without the caller asking for them to be dropped.
	ErrActiveSessions = errors.New("connection has active sessions")
	// ErrCredentialsMasked reports a credential reveal refused by the
	// organization's mask-on-edit setting.
	ErrCredentialsMasked = errors.New("connection credentials are masked for this organization")

	// ErrSQLiteTargetDisabled reports a SQLite file target refused by instance
	// policy.
	ErrSQLiteTargetDisabled = execution.ErrSQLiteTargetDisabled
	// ErrSQLiteInMemoryTargetDisabled reports an in-memory SQLite target
	// refused by instance policy.
	ErrSQLiteInMemoryTargetDisabled = execution.ErrSQLiteInMemoryTargetDisabled
)

// ActiveSessionsReason distinguishes the connection changes that invalidate
// live target sessions, so a caller can tell the operator which change is
// waiting on them.
type ActiveSessionsReason string

// Reasons a connection change would invalidate live sessions.
const (
	ActiveSessionsDSNRotation ActiveSessionsReason = "dsn_rotation"
	ActiveSessionsScopeChange ActiveSessionsReason = "default_scope_change"
)

// ActiveSessionsError reports a connection change refused because live
// sessions would be invalidated by it and the caller did not ask for them to
// be dropped.
type ActiveSessionsError struct {
	Reason   ActiveSessionsReason
	Sessions int
}

// Error implements error.
func (e *ActiveSessionsError) Error() string { return ErrActiveSessions.Error() }

// Unwrap lets callers match with [ErrActiveSessions].
func (e *ActiveSessionsError) Unwrap() error { return ErrActiveSessions }

// SealError reports a connection secret that could not be encrypted. It
// carries the underlying cause so a caller can report why the keyring refused
// the value.
type SealError struct {
	Err error
}

// Error implements error.
func (e *SealError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying keyring error.
func (e *SealError) Unwrap() error { return e.Err }
