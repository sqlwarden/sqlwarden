package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/response"
)

func (s *fakeStore) ListInstanceAdminsPage(_ context.Context, params database.ListInstanceAdminsParams) (response.Paginated[database.InstanceAdmin], error) {
	var items []database.InstanceAdmin
	for id := range s.admins {
		items = append(items, database.InstanceAdmin{AccountID: id})
	}
	return response.Paginated[database.InstanceAdmin]{Items: items, Page: params.Page, PageSize: params.PageSize, Total: len(items)}, nil
}

func (s *fakeStore) InsertInstanceAdmin(_ context.Context, accountID int64) error {
	s.admins[accountID] = true
	return nil
}

func (s *fakeStore) RemoveInstanceAdmin(_ context.Context, accountID int64) error {
	if len(s.admins) <= 1 {
		return database.ErrLastInstanceAdmin
	}
	delete(s.admins, accountID)
	return nil
}

func (s *fakeStore) ListOrganizationsPage(_ context.Context, params database.ListOrganizationsParams) (response.Paginated[database.OrganizationListItem], error) {
	var items []database.OrganizationListItem
	for slug, org := range s.orgs {
		if params.Slug != "" && params.Slug != slug {
			continue
		}
		items = append(items, database.OrganizationListItem{ID: org.ID, Slug: org.Slug, Name: org.Name})
	}
	return response.Paginated[database.OrganizationListItem]{Items: items, Page: params.Page, PageSize: params.PageSize, Total: len(items)}, nil
}

func (s *fakeStore) GetOrgBySlug(_ context.Context, slug string) (database.Organization, bool, error) {
	org, ok := s.orgs[slug]
	return org, ok, nil
}

func (s *fakeStore) ListAccountsPage(_ context.Context, params database.ListAccountsParams) (response.Paginated[database.Account], error) {
	var items []database.Account
	for _, a := range s.accounts {
		items = append(items, a)
	}
	return response.Paginated[database.Account]{Items: items, Page: params.Page, PageSize: params.PageSize, Total: len(items)}, nil
}

func TestInstanceAdministrationRequiresInstanceAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	team := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectTeam, ID: alice}, InstanceAdmin: true}
	for name, p := range map[string]access.Principal{"non-admin account": account(bob), "team subject": team, "anonymous": {}} {
		calls := map[string]error{}
		calls["RequireInstanceAdmin"] = h.svc.RequireInstanceAdmin(ctx, p)
		_, calls["ListInstanceAdmins"] = h.svc.ListInstanceAdmins(ctx, p, identity.ListQuery{})
		_, calls["ListOrganizations"] = h.svc.ListOrganizations(ctx, p, identity.ListQuery{}, "")
		_, calls["ListInstanceAccounts"] = h.svc.ListInstanceAccounts(ctx, p, identity.ListQuery{})
		_, calls["CreateInstanceAccount"] = h.svc.CreateInstanceAccount(ctx, p, identity.CreateAccountInput{Name: "N", Email: "new@example.com", Password: "long-enough"})
		_, calls["AddInstanceAdmin"] = h.svc.AddInstanceAdmin(ctx, p, "bob@example.com")
		calls["RemoveInstanceAdmin"] = h.svc.RemoveInstanceAdmin(ctx, p, alice)
		for method, err := range calls {
			if !errors.Is(err, identity.ErrForbidden) {
				t.Errorf("%s: %s err = %v", name, method, err)
			}
		}
	}
	if h.store.admins[bob] || !h.store.admins[alice] {
		t.Fatalf("denied calls changed admins: %v", h.store.admins)
	}
	if _, exists, _ := h.store.GetAccountByEmail(ctx, "new@example.com"); exists {
		t.Fatal("denied call created an account")
	}
	if len(h.events) != 0 {
		t.Fatalf("denied calls audited: %+v", h.events)
	}
}

func TestInstanceAdminAddAndRemove(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.svc.RequireInstanceAdmin(ctx, admin(alice)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.AddInstanceAdmin(ctx, admin(alice), ""); !errors.Is(err, identity.ErrValidation) {
		t.Fatalf("empty email err = %v", err)
	}
	if _, err := h.svc.AddInstanceAdmin(ctx, admin(alice), "nobody@example.com"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("unknown email err = %v", err)
	}
	added, err := h.svc.AddInstanceAdmin(ctx, admin(alice), "bob@example.com")
	if err != nil || added.ID != bob || !h.store.admins[bob] {
		t.Fatalf("added = %+v, %v", added, err)
	}
	page, err := h.svc.ListInstanceAdmins(ctx, admin(alice), identity.ListQuery{Page: 1, PageSize: 25})
	if err != nil || page.Total != 2 {
		t.Fatalf("page = %+v, %v", page, err)
	}

	if err := h.svc.RemoveInstanceAdmin(ctx, admin(alice), bob); err != nil {
		t.Fatal(err)
	}
	if h.store.admins[bob] {
		t.Fatal("bob is still an admin")
	}
	if len(h.events) != 2 || h.events[0].Action != identity.ActionInstanceAdminAdded || h.events[1].Action != identity.ActionInstanceAdminRemoved ||
		h.events[1].ResourceID != "2" || h.events[1].Actor.SubjectID == nil || *h.events[1].Actor.SubjectID != alice {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestLastInstanceAdminCannotBeRemoved(t *testing.T) {
	h := newHarness(t)
	err := h.svc.RemoveInstanceAdmin(context.Background(), admin(alice), alice)
	if !errors.Is(err, identity.ErrLastInstanceAdmin) {
		t.Fatalf("err = %v", err)
	}
	if !h.store.admins[alice] {
		t.Fatal("last admin was removed")
	}
	if len(h.events) != 0 {
		t.Fatalf("blocked removal audited: %+v", h.events)
	}
}

func TestCreateInstanceAccount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	cases := map[string]struct {
		in    identity.CreateAccountInput
		field string
		want  string
	}{
		"missing name":   {identity.CreateAccountInput{Email: "x@example.com", Password: "long-enough"}, "name", "Name is required."},
		"missing email":  {identity.CreateAccountInput{Name: "X", Password: "long-enough"}, "email", "Email is required."},
		"invalid email":  {identity.CreateAccountInput{Name: "X", Email: "not-an-email", Password: "long-enough"}, "email", "Enter a valid email address."},
		"short password": {identity.CreateAccountInput{Name: "X", Email: "x@example.com", Password: "short"}, "password", "Password must be at least 8 characters."},
		"taken email":    {identity.CreateAccountInput{Name: "X", Email: "bob@example.com", Password: "long-enough"}, "email", "An account with this email already exists."},
	}
	for name, tc := range cases {
		_, err := h.svc.CreateInstanceAccount(ctx, admin(alice), tc.in)
		var validation *identity.ValidationError
		if !errors.As(err, &validation) || validation.Fields[tc.field] != tc.want {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	created, err := h.svc.CreateInstanceAccount(ctx, admin(alice), identity.CreateAccountInput{Name: "  Carol  ", Email: " carol@example.com ", Password: "long-enough"})
	if err != nil || created.Name != "Carol" || created.Email != "carol@example.com" || created.Password == nil || *created.Password == "long-enough" {
		t.Fatalf("created = %+v, %v", created, err)
	}
	if len(h.events) != 1 || h.events[0].Action != identity.ActionInstanceAccountAdded {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestInstanceListsForAdmin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.store.orgs["acme"] = database.Organization{ID: 1, Slug: "acme", CreatedAt: time.Now()}
	h.store.orgs["other"] = database.Organization{ID: 2, Slug: "other", CreatedAt: time.Now()}

	orgs, err := h.svc.ListOrganizations(ctx, admin(alice), identity.ListQuery{Page: 1, PageSize: 25}, "acme")
	if err != nil || orgs.Total != 1 {
		t.Fatalf("orgs = %+v, %v", orgs, err)
	}
	accounts, err := h.svc.ListInstanceAccounts(ctx, admin(alice), identity.ListQuery{Page: 1, PageSize: 25})
	if err != nil || accounts.Total != 2 {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
}
