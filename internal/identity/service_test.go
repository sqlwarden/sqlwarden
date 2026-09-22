package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/password"
)

type identityStore struct {
	configured bool
	byID       map[int64]database.Account
	byEmail    map[string]database.Account
	nextID     int64
}

func (s *identityStore) HasAnyInstanceAdmin(context.Context) (bool, error) { return s.configured, nil }
func (s *identityStore) AccountByEmail(_ context.Context, email string) (database.Account, bool, error) {
	account, ok := s.byEmail[email]
	return account, ok, nil
}
func (s *identityStore) Account(_ context.Context, id int64) (database.Account, bool, error) {
	account, ok := s.byID[id]
	return account, ok, nil
}
func (s *identityStore) InsertAccount(_ context.Context, email, name string, hashed *string) (database.Account, error) {
	s.nextID++
	account := database.Account{ID: s.nextID, Email: email, Name: name, Password: hashed, IsActive: true}
	s.byID[account.ID], s.byEmail[email] = account, account
	return account, nil
}
func (s *identityStore) UpdateAccountName(_ context.Context, id int64, name string) (database.Account, error) {
	account := s.byID[id]
	account.Name = name
	s.byID[id], s.byEmail[account.Email] = account, account
	return account, nil
}
func (s *identityStore) UpdateAccountPassword(_ context.Context, id int64, hashed string) error {
	account := s.byID[id]
	account.Password = &hashed
	s.byID[id], s.byEmail[account.Email] = account, account
	return nil
}

func newIdentityFixture(t *testing.T) (*identity.Service, *identityStore) {
	t.Helper()
	service, store, _ := newAuditedIdentityFixture(t)
	return service, store
}

func newAuditedIdentityFixture(t *testing.T) (*identity.Service, *identityStore, *recordingAudit) {
	t.Helper()
	store := &identityStore{configured: true, byID: map[int64]database.Account{}, byEmail: map[string]database.Account{}}
	provider := identity.NewCoreProvider(store)
	recorder := &recordingAudit{}
	return identity.NewService(store, provider, recorder), store, recorder
}

// recordingAudit captures the audit intent a use case emitted.
type recordingAudit struct {
	events []audit.Event
}

func (r *recordingAudit) Write(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingAudit) outcomes(action string) []string {
	var outcomes []string
	for _, event := range r.events {
		if event.Action == action {
			outcomes = append(outcomes, event.Outcome)
		}
	}
	return outcomes
}

func TestRegisterRequiresConfiguredInstanceAndUniqueEmail(t *testing.T) {
	t.Parallel()
	service, store := newIdentityFixture(t)
	store.configured = false
	_, err := service.Register(context.Background(), identity.RegisterInput{Email: "user@example.com", Name: "User", Password: "password123"})
	if !errors.Is(err, identity.ErrSetupIncomplete) {
		t.Fatalf("error = %v, want ErrSetupIncomplete", err)
	}

	store.configured = true
	if _, err := service.Register(context.Background(), identity.RegisterInput{Email: "user@example.com", Name: "User", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	_, err = service.Register(context.Background(), identity.RegisterInput{Email: "user@example.com", Name: "Other", Password: "password123"})
	if !errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("duplicate error = %v, want ErrEmailTaken", err)
	}
}

func TestRegisterValidatesIdentityInputs(t *testing.T) {
	t.Parallel()
	service, _ := newIdentityFixture(t)
	for _, test := range []struct {
		input identity.RegisterInput
		want  error
	}{
		{input: identity.RegisterInput{Email: "invalid", Name: "User", Password: "password123"}, want: identity.ErrInvalidEmail},
		{input: identity.RegisterInput{Email: "user@example.com", Name: " ", Password: "password123"}, want: identity.ErrInvalidName},
		{input: identity.RegisterInput{Email: "user@example.com", Name: "User", Password: "short"}, want: identity.ErrWeakPassword},
	} {
		if _, err := service.Register(context.Background(), test.input); !errors.Is(err, test.want) {
			t.Errorf("registration error = %v, want %v", err, test.want)
		}
	}
}

func TestAuthenticateHidesAccountState(t *testing.T) {
	t.Parallel()
	service, store := newIdentityFixture(t)
	hashed, err := password.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	account := database.Account{ID: 1, Email: "user@example.com", Name: "User", Password: &hashed, IsActive: false}
	store.byID[account.ID], store.byEmail[account.Email] = account, account

	for _, password := range []string{"password123", "wrong-password"} {
		_, err := service.AuthenticateWithPassword(context.Background(), account.Email, password)
		if !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("password %q error = %v, want ErrInvalidCredentials", password, err)
		}
	}
}

func TestChangePasswordVerifiesCurrentCredential(t *testing.T) {
	t.Parallel()
	service, store := newIdentityFixture(t)
	hashed, err := password.Hash("old-password")
	if err != nil {
		t.Fatal(err)
	}
	account := database.Account{ID: 1, Email: "user@example.com", Name: "User", Password: &hashed, IsActive: true}
	store.byID[account.ID], store.byEmail[account.Email] = account, account

	if err := service.ChangePassword(context.Background(), account.ID, "wrong", "new-password"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("wrong current password error = %v", err)
	}
	if err := service.ChangePassword(context.Background(), account.ID, "old-password", "new-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateWithPassword(context.Background(), account.Email, "new-password"); err != nil {
		t.Fatalf("authenticate with new password: %v", err)
	}
}

func TestAuthenticationAuditsBothOutcomes(t *testing.T) {
	t.Parallel()
	service, _, recorder := newAuditedIdentityFixture(t)
	account, err := service.Register(context.Background(), identity.RegisterInput{
		Email: "audited@example.com", Name: "Audited", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.AuthenticateWithPassword(context.Background(), account.Email, "password123"); err != nil {
		t.Fatal(err)
	}
	_, err = service.AuthenticateWithPassword(context.Background(), account.Email, "wrong-password")
	if !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("error = %v, want %v", err, identity.ErrInvalidCredentials)
	}

	outcomes := recorder.outcomes(identity.ActionAuthenticate)
	if len(outcomes) != 2 || outcomes[0] != audit.OutcomeSuccess || outcomes[1] != audit.OutcomeFailure {
		t.Fatalf("authentication audit outcomes = %v, want [success failure]", outcomes)
	}
	if registered := recorder.outcomes(identity.ActionRegister); len(registered) != 1 {
		t.Fatalf("registration audit outcomes = %v, want one event", registered)
	}
	for _, event := range recorder.events {
		for key, value := range event.Metadata {
			if value == "password123" || value == "wrong-password" {
				t.Fatalf("audit metadata %q leaked a credential", key)
			}
		}
	}
}
