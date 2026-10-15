package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
)

type memoryAccounts map[string]database.Account

func (m memoryAccounts) GetAccountByEmail(_ context.Context, email string) (database.Account, bool, error) {
	a, ok := m[email]
	return a, ok, nil
}

func signInFixture(t *testing.T) memoryAccounts {
	t.Helper()
	hash, err := password.Hash("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	return memoryAccounts{
		"active@example.com":   {ID: 1, Email: "active@example.com", Password: &hash, IsActive: true},
		"inactive@example.com": {ID: 2, Email: "inactive@example.com", Password: &hash},
		"nohash@example.com":   {ID: 3, Email: "nohash@example.com", IsActive: true},
	}
}

func requireSignInReason(t *testing.T, err error, reason string) {
	t.Helper()
	var credErr *CredentialError
	if !errors.As(err, &credErr) || credErr.Reason != reason {
		t.Fatalf("err = %v, want credential error %q", err, reason)
	}
}

func TestSignInEnabled(t *testing.T) {
	if !PasswordSignIn(signInFixture(t)).Enabled() {
		t.Fatal("password sign-in must be enabled")
	}
	if SignInUnavailable.Enabled() {
		t.Fatal("unavailable sign-in must be disabled")
	}
}

func TestPasswordMethodBeginAcceptsValidCredentials(t *testing.T) {
	method := NewPasswordMethod(signInFixture(t))
	subject, err := method.Begin(context.Background(), Credentials{Email: " active@example.com ", Password: "correct-horse"})
	if err != nil {
		t.Fatal(err)
	}
	if subject.Account.ID != 1 {
		t.Fatalf("account id = %d", subject.Account.ID)
	}
}

func TestPasswordMethodBeginRejectsWithSingleReason(t *testing.T) {
	method := NewPasswordMethod(signInFixture(t))
	cases := map[string]Credentials{
		"unknown email":  {Email: "ghost@example.com", Password: "correct-horse"},
		"nil hash":       {Email: "nohash@example.com", Password: "correct-horse"},
		"inactive":       {Email: "inactive@example.com", Password: "correct-horse"},
		"wrong password": {Email: "active@example.com", Password: "wrong"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := method.Begin(context.Background(), c)
			requireSignInReason(t, err, "invalid_credentials")
		})
	}
}

func TestPasswordMethodBeginRequiresCredentials(t *testing.T) {
	method := NewPasswordMethod(signInFixture(t))
	for _, c := range []Credentials{{Password: "x"}, {Email: "active@example.com"}, {Email: "  ", Password: "x"}} {
		_, err := method.Begin(context.Background(), c)
		requireSignInReason(t, err, "credentials_required")
	}
}

func TestPasswordMethodComplete(t *testing.T) {
	info, err := NewPasswordMethod(signInFixture(t)).Complete(context.Background(), Subject{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Kind != access.CredentialSession || info.Method != MethodPassword || info.Assurance != access.AAL1 || info.AuthTime.IsZero() {
		t.Fatalf("credential = %+v", info)
	}
}

func TestDefaultFactorAndSignInPolicies(t *testing.T) {
	if got := NoFactors.Required(context.Background(), Subject{}, access.RequestAttributes{}); len(got) != 0 {
		t.Fatalf("factors = %v", got)
	}
	decision, err := AllowAllSignIn.Evaluate(context.Background(), Subject{}, access.RequestAttributes{})
	if err != nil || decision != access.Allow() {
		t.Fatalf("decision = %+v, err = %v", decision, err)
	}
}
