package identity

import "errors"

// Identity application errors. Transport layers map these to status codes and
// user-facing messages; none of them carry credential material.
var (
	// ErrInvalidCredentials reports a failed authentication. It is returned
	// for an unknown identifier, a wrong secret, a disabled account, and an
	// account with no usable credential, so a caller cannot probe which.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnsupportedMethod reports an authentication method this provider does
	// not implement.
	ErrUnsupportedMethod = errors.New("unsupported authentication method")
	// ErrSetupIncomplete reports registration attempted before the instance
	// has an administrator.
	ErrSetupIncomplete = errors.New("instance setup is not complete")
	// ErrEmailTaken reports an account that already exists for an email.
	ErrEmailTaken = errors.New("email already registered")
	// ErrPasswordUnavailable reports a password change on an account that
	// authenticates through an external identity provider.
	ErrPasswordUnavailable = errors.New("password authentication unavailable for account")
	// ErrAccountNotFound reports an account that does not exist or is disabled.
	ErrAccountNotFound = errors.New("account not found")
	// ErrInvalidEmail reports a malformed local account identifier.
	ErrInvalidEmail = errors.New("invalid account email")
	// ErrInvalidName reports an empty display name.
	ErrInvalidName = errors.New("invalid account name")
	// ErrWeakPassword reports a local password below the core minimum length.
	ErrWeakPassword = errors.New("password must be at least 8 characters")
)
