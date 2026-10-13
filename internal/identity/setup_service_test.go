package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/token"
)

type fakeFirstRun struct {
	store *fakeStore
	calls []identity.FirstRun
	err   error
}

func (f *fakeFirstRun) CreateFirstRun(_ context.Context, in identity.FirstRun) (identity.FirstRunRecords, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return identity.FirstRunRecords{}, f.err
	}
	f.store.nextAccount++
	acct := database.Account{ID: f.store.nextAccount, Email: in.Email, Name: in.Name, Password: in.PasswordHash, IsActive: true}
	f.store.accounts[acct.ID] = acct
	f.store.admins[acct.ID] = true
	f.store.adminExists = true
	org := database.Organization{ID: 500, Slug: in.OrgSlug, Name: in.OrgName}
	f.store.orgs[org.Slug] = org
	return identity.FirstRunRecords{
		Account: acct, Org: org,
		AuthSession: database.AuthSession{ID: "setup-session", AccountID: acct.ID, ExpiresAt: in.SessionExpiresAt},
	}, nil
}

type fakeCache struct{ invalidated []int64 }

func (c *fakeCache) InvalidateOrgPolicy(orgID int64) { c.invalidated = append(c.invalidated, orgID) }

type setupHarness struct {
	*harness
	svc      *identity.Service
	firstRun *fakeFirstRun
	cache    *fakeCache
}

func newSetupHarness(t *testing.T, strategy identity.SetupStrategy) *setupHarness {
	t.Helper()
	h := newHarness(t)
	h.store.adminExists = false
	h.store.admins = map[int64]bool{}
	s := &setupHarness{harness: h, firstRun: &fakeFirstRun{store: h.store}, cache: &fakeCache{}}
	s.svc = identity.New(identity.Deps{
		Store:    h.store,
		Setup:    strategy,
		FirstRun: s.firstRun,
		Cache:    s.cache,
		Tokens:   fixedTTL(15 * time.Minute),
		Secret:   secret,
		Audit: audit.WriterFunc(func(_ context.Context, e audit.Event) error {
			h.events = append(h.events, e)
			return nil
		}),
		Now: func() time.Time { return h.now },
	})
	return s
}

var formInput = identity.SetupInput{
	Name: "Admin", Email: "admin@example.com", Password: "admin-password",
	OrganizationName: "Acme Corp", OrganizationSlug: "acme",
}

func TestSetupCreatesAdminOrgAndSession(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	ctx := context.Background()

	result, err := s.svc.Setup(ctx, formInput, attrs)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.firstRun.calls) != 1 {
		t.Fatalf("first run calls = %d", len(s.firstRun.calls))
	}
	in := s.firstRun.calls[0]
	if in.Email != "admin@example.com" || in.Name != "Admin" || in.OrgSlug != "acme" || in.OrgName != "Acme Corp" {
		t.Fatalf("first run = %+v", in)
	}
	if in.PasswordHash == nil || *in.PasswordHash == formInput.Password {
		t.Fatal("password was not hashed")
	}
	if ok, err := password.Matches(formInput.Password, *in.PasswordHash); err != nil || !ok {
		t.Fatalf("hash does not match: %v", err)
	}
	if !in.SessionExpiresAt.Equal(s.now.Add(identity.RefreshTokenTTL)) || in.UserAgent != "test-agent" || in.IPAddress != "192.0.2.7" {
		t.Fatalf("session fields = %+v", in)
	}
	if in.Auth.Method != identity.MethodPassword || in.Auth.Assurance != string(access.AAL1) {
		t.Fatalf("auth = %+v", in.Auth)
	}

	if result.Org.ID != 500 || result.AuthSessionID != "setup-session" || result.Method != identity.MethodPassword {
		t.Fatalf("result = %+v", result)
	}
	claims, err := token.Verify(result.AccessToken, secret)
	if err != nil || claims.AuthSessionID != "setup-session" || claims.Email != "admin@example.com" {
		t.Fatalf("claims = %+v, %v", claims, err)
	}
	if len(s.cache.invalidated) != 1 || s.cache.invalidated[0] != 500 {
		t.Fatalf("invalidated = %v", s.cache.invalidated)
	}
	if len(s.events) != 1 {
		t.Fatalf("events = %+v", s.events)
	}
	e := s.events[0]
	if e.Action != audit.ActionSetupCompleted || e.OrgID == nil || *e.OrgID != 500 || e.Metadata["client_ip"] != "192.0.2.7" ||
		e.Actor.CredentialID != "setup-session" {
		t.Fatalf("event = %+v", e)
	}
}

func TestSetupRefusesSecondRun(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	ctx := context.Background()
	if _, err := s.svc.Setup(ctx, formInput, attrs); err != nil {
		t.Fatal(err)
	}
	second := formInput
	second.Email = "second@example.com"
	second.OrganizationSlug = "second"
	if _, err := s.svc.Setup(ctx, second, attrs); !errors.Is(err, identity.ErrAlreadyConfigured) {
		t.Fatalf("second setup err = %v", err)
	}
	if len(s.firstRun.calls) != 1 {
		t.Fatalf("second setup reached the store: %d calls", len(s.firstRun.calls))
	}
	state, err := s.svc.SetupStatus(ctx)
	if err != nil || !state.Configured || !state.RequiresInput {
		t.Fatalf("state = %+v, %v", state, err)
	}
}

func TestSetupStoreRaceReportsAlreadyConfigured(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	s.firstRun.err = identity.ErrAlreadyConfigured
	if _, err := s.svc.Setup(context.Background(), formInput, attrs); !errors.Is(err, identity.ErrAlreadyConfigured) {
		t.Fatalf("err = %v", err)
	}
	if len(s.cache.invalidated) != 0 || len(s.events) != 0 {
		t.Fatalf("failed setup had side effects: %v %+v", s.cache.invalidated, s.events)
	}
}

func TestSetupValidationPrecedesConfiguredCheck(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	s.store.adminExists = true
	_, err := s.svc.Setup(context.Background(), identity.SetupInput{}, attrs)
	var validation *identity.ValidationError
	if !errors.As(err, &validation) || validation.Fields["name"] == "" || validation.Fields["email"] == "" {
		t.Fatalf("err = %v", err)
	}
}

func TestSetupRejectsTakenSlugForForm(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	s.store.orgs["acme"] = database.Organization{ID: 1, Slug: "acme"}
	_, err := s.svc.Setup(context.Background(), formInput, attrs)
	var validation *identity.ValidationError
	if !errors.As(err, &validation) || validation.Fields["organization_slug"] != "An organization with this slug already exists." {
		t.Fatalf("err = %v", err)
	}
	if len(s.firstRun.calls) != 0 {
		t.Fatal("taken slug reached the store")
	}
}

func TestSetupUniqueViolationIsFieldError(t *testing.T) {
	s := newSetupHarness(t, identity.FormSetup)
	s.firstRun.err = errors.New("UNIQUE constraint failed: accounts.email")
	_, err := s.svc.Setup(context.Background(), formInput, attrs)
	var validation *identity.ValidationError
	if !errors.As(err, &validation) || validation.Fields["email"] != "An account or organization with these details already exists." {
		t.Fatalf("err = %v", err)
	}
}

func TestDesktopSetupUsesLocalStrategy(t *testing.T) {
	s := newSetupHarness(t, identity.LocalSetup)
	ctx := context.Background()
	s.store.orgs[identity.LocalOrganizationSlug] = database.Organization{ID: 1, Slug: identity.LocalOrganizationSlug}

	state, err := s.svc.SetupStatus(ctx)
	if err != nil || state.Configured || state.RequiresInput {
		t.Fatalf("state = %+v, %v", state, err)
	}
	result, err := s.svc.Setup(ctx, identity.SetupInput{}, attrs)
	if err != nil {
		t.Fatal(err)
	}
	in := s.firstRun.calls[0]
	if in.Email != identity.LocalAccountEmail || in.PasswordHash != nil || in.Auth.Method != identity.MethodLocal {
		t.Fatalf("first run = %+v", in)
	}
	if !strings.HasPrefix(in.OrgSlug, identity.LocalOrganizationSlug+"-") || in.OrgName != identity.LocalOrganizationName {
		t.Fatalf("slug fallback = %q %q", in.OrgSlug, in.OrgName)
	}
	if result.Method != identity.MethodLocal || result.AccessToken == "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := s.svc.Setup(ctx, identity.SetupInput{}, attrs); !errors.Is(err, identity.ErrAlreadyConfigured) {
		t.Fatalf("second desktop setup err = %v", err)
	}
}

func TestSetupWithoutFirstRunStoreFails(t *testing.T) {
	h := newHarness(t)
	h.store.adminExists = false
	if _, err := h.svc.Setup(context.Background(), formInput, attrs); err == nil {
		t.Fatal("setup without a first-run store succeeded")
	}
}
