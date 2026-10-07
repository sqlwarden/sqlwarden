package identity

import (
	"context"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
)

type Credentials struct {
	Email    string
	Password string
}

// Subject is an account whose primary credential has been verified.
type Subject struct {
	Account database.Account
}

// Method verifies a primary credential. Begin authenticates the credential and
// Complete describes the resulting session credential.
type Method interface {
	Name() string
	Begin(ctx context.Context, c Credentials) (Subject, error)
	Complete(ctx context.Context, s Subject) (access.CredentialInfo, error)
}

type Factor interface {
	Kind() string
	Challenge(ctx context.Context, s Subject) error
	Verify(ctx context.Context, s Subject, response string) error
}

// FactorPolicy returns the additional factors a sign-in must satisfy.
type FactorPolicy interface {
	Required(ctx context.Context, s Subject, attrs access.RequestAttributes) []Factor
}

// SignInPolicy decides whether a verified subject may start a session.
type SignInPolicy interface {
	Evaluate(ctx context.Context, s Subject, attrs access.RequestAttributes) (access.Decision, error)
}

// SignInStrategy composes the method, factors, and policy a profile offers.
type SignInStrategy interface {
	Enabled() bool
	Method() Method
	Factors() FactorPolicy
	Policy() SignInPolicy
}

type AccountLookup interface {
	GetAccountByEmail(ctx context.Context, email string) (database.Account, bool, error)
}

// dummyHash lets an unknown email cost the same as a known one.
var dummyHash = mustHash("sqlwarden-timing-equalizer")

func mustHash(s string) string {
	h, err := password.Hash(s)
	if err != nil {
		panic(err)
	}
	return h
}

type passwordMethod struct {
	accounts AccountLookup
	now      func() time.Time
}

func NewPasswordMethod(accounts AccountLookup) Method {
	return passwordMethod{accounts: accounts, now: time.Now}
}

func (passwordMethod) Name() string { return MethodPassword }

func (m passwordMethod) Begin(ctx context.Context, c Credentials) (Subject, error) {
	email := strings.TrimSpace(c.Email)
	if email == "" || c.Password == "" {
		return Subject{}, &CredentialError{Reason: "credentials_required"}
	}
	account, found, err := m.accounts.GetAccountByEmail(ctx, email)
	if err != nil {
		return Subject{}, err
	}
	hash := dummyHash
	if found && account.Password != nil {
		hash = *account.Password
	}
	matches, err := password.Matches(c.Password, hash)
	if err != nil {
		return Subject{}, err
	}
	if !found || account.Password == nil || !account.IsActive || !matches {
		return Subject{}, &CredentialError{Reason: "invalid_credentials"}
	}
	return Subject{Account: account}, nil
}

func (m passwordMethod) Complete(context.Context, Subject) (access.CredentialInfo, error) {
	return access.CredentialInfo{
		Kind: access.CredentialSession, Method: MethodPassword,
		Assurance: access.AAL1, AuthTime: m.now(),
	}, nil
}

type noFactors struct{}

func (noFactors) Required(context.Context, Subject, access.RequestAttributes) []Factor { return nil }

type allowAll struct{}

func (allowAll) Evaluate(context.Context, Subject, access.RequestAttributes) (access.Decision, error) {
	return access.Allow(), nil
}

var (
	NoFactors      FactorPolicy = noFactors{}
	AllowAllSignIn SignInPolicy = allowAll{}
)

type signIn struct {
	method Method
}

func PasswordSignIn(accounts AccountLookup) SignInStrategy {
	return signIn{method: NewPasswordMethod(accounts)}
}

func (s signIn) Enabled() bool       { return s.method != nil }
func (s signIn) Method() Method      { return s.method }
func (signIn) Factors() FactorPolicy { return NoFactors }
func (signIn) Policy() SignInPolicy  { return AllowAllSignIn }

var SignInUnavailable SignInStrategy = signIn{}
