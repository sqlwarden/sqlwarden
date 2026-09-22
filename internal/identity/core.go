package identity

import (
	"context"
	"strings"

	"github.com/sqlwarden/internal/password"
)

// CoreProvider is the Community authentication path. It verifies a local
// password credential against the accounts table and returns the stable
// subject for the account.
//
// Federated login (OIDC, SAML) is not implemented in core: the only OIDC state
// core persists is an organization's identity-provider configuration row, and
// no core endpoint consumes an assertion. CoreProvider therefore reports
// [ErrUnsupportedMethod] for federated methods, and an edition decorator
// supplies federation by wrapping it.
type CoreProvider struct {
	store Store
}

// NewCoreProvider returns the core password provider reading through store.
func NewCoreProvider(store Store) *CoreProvider {
	return &CoreProvider{store: store}
}

// Authenticate implements [Provider].
func (p *CoreProvider) Authenticate(ctx context.Context, request AuthenticationRequest) (Subject, error) {
	if request.Method != AuthenticationPassword {
		return Subject{}, ErrUnsupportedMethod
	}

	email := strings.TrimSpace(request.Identifier)
	if email == "" || request.Secret == "" {
		return Subject{}, ErrInvalidCredentials
	}

	account, found, err := p.store.AccountByEmail(ctx, email)
	if err != nil {
		return Subject{}, err
	}
	if !found || account.Password == nil || !account.IsActive {
		return Subject{}, ErrInvalidCredentials
	}

	match, err := password.Matches(request.Secret, *account.Password)
	if err != nil {
		return Subject{}, err
	}
	if !match {
		return Subject{}, ErrInvalidCredentials
	}

	return Subject{AccountID: account.ID, Email: account.Email, Name: account.Name}, nil
}

var _ Provider = (*CoreProvider)(nil)
