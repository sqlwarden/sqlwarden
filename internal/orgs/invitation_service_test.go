package orgs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
)

const (
	inviteeID       int64 = 30
	inactiveID      int64 = 31
	pendingExisting       = "inv-existing"
	pendingNew            = "inv-new"
	expiredInvite         = "inv-expired"
	acceptedInvite        = "inv-accepted"
	inactiveInvite        = "inv-inactive"
)

type fakeBaseURL string

func (b fakeBaseURL) BaseURL(context.Context) (string, error) { return string(b), nil }

type sentMail struct {
	recipient string
	data      map[string]any
	templates []string
}

type fakeMailer struct {
	sent []sentMail
	err  error
}

func (m *fakeMailer) SendOnce(recipient string, data any, templates ...string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{recipient: recipient, data: data.(map[string]any), templates: templates})
	return nil
}

type acceptCall struct {
	invitationID string
	accountID    int64
	newAccount   *orgs.InvitedAccount
}

type fakeInvitationStore struct {
	invitations map[string]database.OrganizationInvitation
	accounts    map[int64]database.Account
	insertErr   error
	acceptErr   error

	inserted   []database.OrganizationInvitation
	rotated    []string
	revoked    []string
	deliveries map[string]string
	accepted   []acceptCall
}

func newFakeInvitationStore() *fakeInvitationStore {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	acceptedAt := past
	invite := func(id, email string, expires time.Time) database.OrganizationInvitation {
		return database.OrganizationInvitation{ID: id, OrgID: testOrg, Email: email, TokenHash: token.Hash("tok-" + id), ExpiresAt: expires}
	}
	accepted := invite(acceptedInvite, "done@example.com", future)
	accepted.AcceptedAt = &acceptedAt
	s := &fakeInvitationStore{
		invitations: map[string]database.OrganizationInvitation{},
		accounts: map[int64]database.Account{
			ownerID:    {ID: ownerID, Email: "owner@example.com", Name: "Owner", IsActive: true},
			readerID:   {ID: readerID, Email: "reader@example.com", Name: "Reader", IsActive: true},
			inviteeID:  {ID: inviteeID, Email: "invitee@example.com", Name: "Invitee", IsActive: true},
			inactiveID: {ID: inactiveID, Email: "inactive@example.com", Name: "Inactive"},
		},
		deliveries: map[string]string{},
	}
	for _, inv := range []database.OrganizationInvitation{
		invite(pendingExisting, "invitee@example.com", future),
		invite(pendingNew, "new@example.com", future),
		invite(expiredInvite, "late@example.com", past),
		invite(inactiveInvite, "inactive@example.com", future),
		accepted,
	} {
		s.invitations[inv.ID] = inv
	}
	return s
}

func (s *fakeInvitationStore) writes() int {
	return len(s.inserted) + len(s.rotated) + len(s.revoked) + len(s.deliveries) + len(s.accepted)
}

func (s *fakeInvitationStore) ListOrganizationInvitationsPage(_ context.Context, params database.ListOrganizationInvitationsParams) (response.Paginated[database.OrganizationInvitationListItem], error) {
	var out []database.OrganizationInvitationListItem
	for _, inv := range s.invitations {
		if inv.OrgID == params.OrgID {
			out = append(out, database.OrganizationInvitationListItem{OrganizationInvitation: inv})
		}
	}
	return response.Paginated[database.OrganizationInvitationListItem]{Items: out}, nil
}

func (s *fakeInvitationStore) InsertOrganizationInvitation(_ context.Context, orgID int64, email, tokenHash string, invitedBy int64, expiresAt time.Time) (database.OrganizationInvitation, error) {
	if s.insertErr != nil {
		return database.OrganizationInvitation{}, s.insertErr
	}
	inv := database.OrganizationInvitation{ID: "inv-created", OrgID: orgID, Email: email, TokenHash: tokenHash, InvitedByAccountID: &invitedBy, ExpiresAt: expiresAt}
	s.inserted = append(s.inserted, inv)
	return inv, nil
}

func (s *fakeInvitationStore) RotateOrganizationInvitation(_ context.Context, orgID int64, id, tokenHash string, expiresAt time.Time) (database.OrganizationInvitation, bool, error) {
	inv, ok := s.invitations[id]
	if !ok || inv.OrgID != orgID || inv.AcceptedAt != nil || inv.RevokedAt != nil {
		return database.OrganizationInvitation{}, false, nil
	}
	inv.TokenHash, inv.ExpiresAt = tokenHash, expiresAt
	s.invitations[id] = inv
	s.rotated = append(s.rotated, id)
	return inv, true, nil
}

func (s *fakeInvitationStore) RevokeOrganizationInvitation(_ context.Context, orgID int64, id string) (bool, error) {
	inv, ok := s.invitations[id]
	if !ok || inv.OrgID != orgID || inv.AcceptedAt != nil || inv.RevokedAt != nil {
		return false, nil
	}
	s.revoked = append(s.revoked, id)
	return true, nil
}

func (s *fakeInvitationStore) UpdateOrganizationInvitationDelivery(_ context.Context, id, status string) error {
	s.deliveries[id] = status
	return nil
}

func (s *fakeInvitationStore) GetOrganizationInvitationByTokenHash(_ context.Context, tokenHash string) (database.OrganizationInvitation, bool, error) {
	for _, inv := range s.invitations {
		if inv.TokenHash == tokenHash {
			return inv, true, nil
		}
	}
	return database.OrganizationInvitation{}, false, nil
}

func (s *fakeInvitationStore) GetAccount(_ context.Context, id int64) (database.Account, bool, error) {
	a, ok := s.accounts[id]
	return a, ok, nil
}

func (s *fakeInvitationStore) GetAccountByEmail(_ context.Context, email string) (database.Account, bool, error) {
	for _, a := range s.accounts {
		if strings.EqualFold(a.Email, email) {
			return a, true, nil
		}
	}
	return database.Account{}, false, nil
}

func (s *fakeInvitationStore) AcceptOrganizationInvitation(_ context.Context, inv database.OrganizationInvitation, accountID int64, newAccount *orgs.InvitedAccount) (database.Account, error) {
	if s.acceptErr != nil {
		return database.Account{}, s.acceptErr
	}
	s.accepted = append(s.accepted, acceptCall{invitationID: inv.ID, accountID: accountID, newAccount: newAccount})
	if newAccount != nil {
		return database.Account{ID: 99, Email: newAccount.Email, Name: newAccount.Name, IsActive: true}, nil
	}
	return database.Account{ID: accountID}, nil
}

func noInput() (orgs.AcceptInvitationInput, error) { return orgs.AcceptInvitationInput{}, nil }

func input(name, password string) func() (orgs.AcceptInvitationInput, error) {
	return func() (orgs.AcceptInvitationInput, error) {
		return orgs.AcceptInvitationInput{Name: name, Password: password}, nil
	}
}

func invitationCalls(h *harness) map[string]func(access.Principal, int64) error {
	ctx := context.Background()
	return map[string]func(access.Principal, int64) error{
		"ListInvitations": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ListInvitations(ctx, p, org, database.ListOrganizationInvitationsParams{}))
		},
		"CreateInvitation": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.CreateInvitation(ctx, p, org, "fresh@example.com"))
		},
		"ResendInvitation": func(p access.Principal, org int64) error {
			return accesstest.Err(h.svc.ResendInvitation(ctx, p, org, pendingNew))
		},
		"RevokeInvitation": func(p access.Principal, org int64) error {
			return h.svc.RevokeInvitation(ctx, p, org, pendingNew)
		},
	}
}

func TestInvitationManagementRequiresInvitePermission(t *testing.T) {
	h := newHarness(t)
	calls := invitationCalls(h)
	for name, call := range calls {
		accesstest.AssertForbidden(t, call(account(writerID), testOrg), name+" by writer")
		accesstest.AssertForbidden(t, call(account(nonMemberID), testOrg), name+" by non-member")
		accesstest.AssertForbidden(t, call(access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: ownerID}}, testOrg), name+" by service account")
		accesstest.AssertNotFound(t, call(account(ownerID), missingOrg), name+" on missing org")
	}
	if h.invites.writes() != 0 || len(h.mailer.sent) != 0 || len(h.events) != 0 {
		t.Fatal("a denied invitation call had side effects")
	}
	for name, call := range invitationCalls(newHarness(t)) {
		if err := call(account(ownerID), testOrg); err != nil {
			t.Fatalf("%s by owner: %v", name, err)
		}
	}
}

func TestInvitationsDisabledAreNotFound(t *testing.T) {
	h := newHarnessWith(t, orgs.InvitationsDisabled)
	for name, call := range invitationCalls(h) {
		accesstest.AssertNotFound(t, call(account(ownerID), testOrg), name)
	}
	ctx := context.Background()
	accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetInvitation(ctx, "tok-"+pendingNew, 0)), "GetInvitation")
	read := false
	_, err := h.svc.AcceptInvitation(ctx, "tok-"+pendingNew, 0, func() (orgs.AcceptInvitationInput, error) { read = true; return noInput() })
	accesstest.AssertNotFound(t, err, "AcceptInvitation")
	if read || h.invites.writes() != 0 {
		t.Fatal("disabled invitations were processed")
	}
}

func TestCreateInvitationValidation(t *testing.T) {
	cases := []struct {
		email     string
		insertErr error
		want      string
	}{
		{"  ", nil, "Email is required."},
		{"not-an-email", nil, "Enter a valid email address."},
		{"Reader@example.com", nil, "This account is already a member of the organization."},
		{"fresh@example.com", errors.New("UNIQUE constraint failed: organization_invitations.org_id"), "A pending invitation already exists for this email."},
	}
	for _, tc := range cases {
		h := newHarness(t)
		h.invites.insertErr = tc.insertErr
		_, err := h.svc.CreateInvitation(context.Background(), account(ownerID), testOrg, tc.email)
		var v *orgs.ValidationError
		if !errors.As(err, &v) || v.Fields["email"] != tc.want {
			t.Fatalf("CreateInvitation(%q) = %v, want %q", tc.email, err, tc.want)
		}
		if len(h.mailer.sent) != 0 || len(h.events) != 0 {
			t.Fatalf("CreateInvitation(%q) delivered or audited a rejected invitation", tc.email)
		}
	}
}

func TestCreateInvitationDeliversLinkAndAudits(t *testing.T) {
	h := newHarness(t)
	issued, err := h.svc.CreateInvitation(context.Background(), account(ownerID), testOrg, " fresh@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "https://sqlwarden.example/invitations/"
	if !strings.HasPrefix(issued.InviteURL, prefix) {
		t.Fatalf("invite URL = %q", issued.InviteURL)
	}
	plain := strings.TrimPrefix(issued.InviteURL, prefix)
	if len(h.invites.inserted) != 1 {
		t.Fatalf("inserted = %d", len(h.invites.inserted))
	}
	stored := h.invites.inserted[0]
	if stored.Email != "fresh@example.com" || stored.TokenHash != token.Hash(plain) || stored.TokenHash == plain {
		t.Fatalf("stored invitation = %+v", stored)
	}
	if *stored.InvitedByAccountID != ownerID || time.Until(stored.ExpiresAt) < orgs.InvitationTTL-time.Minute {
		t.Fatalf("stored inviter/expiry = %v %v", *stored.InvitedByAccountID, stored.ExpiresAt)
	}
	if len(h.mailer.sent) != 1 {
		t.Fatalf("sent = %d", len(h.mailer.sent))
	}
	mail := h.mailer.sent[0]
	if mail.recipient != "fresh@example.com" || mail.templates[0] != "organization-invitation.tmpl" ||
		mail.data["InviterName"] != "Owner" || mail.data["OrganizationName"] != "Acme" || mail.data["InviteURL"] != issued.InviteURL {
		t.Fatalf("mail = %+v", mail)
	}
	if issued.DeliveryStatus != database.InvitationDeliverySent || issued.Invitation.LastDeliveryStatus != database.InvitationDeliverySent ||
		h.invites.deliveries["inv-created"] != database.InvitationDeliverySent {
		t.Fatalf("delivery = %q, stored %q", issued.DeliveryStatus, h.invites.deliveries["inv-created"])
	}
	if len(h.events) != 1 {
		t.Fatalf("events = %v", h.actions())
	}
	e := h.events[0]
	if e.Action != orgs.ActionInvitationCreated || e.Resource != "invitation" || e.ResourceID != "inv-created" || *e.OrgID != testOrg || len(e.Metadata) != 0 {
		t.Fatalf("event = %+v", e)
	}
}

func TestInvitationDeliveryStatus(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"disabled": {orgs.ErrMailDisabled, database.InvitationDeliveryDisabled},
		"failed":   {errors.New("connection refused"), database.InvitationDeliveryFailed},
	}
	for name, tc := range cases {
		h := newHarness(t)
		h.mailer.err = tc.err
		issued, err := h.svc.CreateInvitation(context.Background(), account(ownerID), testOrg, "fresh@example.com")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if issued.DeliveryStatus != tc.want || h.invites.deliveries["inv-created"] != tc.want || issued.InviteURL == "" {
			t.Fatalf("%s: status = %q stored %q url %q", name, issued.DeliveryStatus, h.invites.deliveries["inv-created"], issued.InviteURL)
		}
	}
}

func TestResendInvitationRotatesToken(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	issued, err := h.svc.ResendInvitation(ctx, account(ownerID), testOrg, pendingNew)
	if err != nil {
		t.Fatal(err)
	}
	plain := issued.InviteURL[strings.LastIndex(issued.InviteURL, "/")+1:]
	if h.invites.invitations[pendingNew].TokenHash != token.Hash(plain) || plain == "tok-"+pendingNew {
		t.Fatal("resend did not rotate the token")
	}
	if len(h.mailer.sent) != 1 || h.invites.deliveries[pendingNew] != database.InvitationDeliverySent {
		t.Fatal("resend did not deliver")
	}
	_, err = h.svc.ResendInvitation(ctx, account(ownerID), testOrg, acceptedInvite)
	accesstest.AssertNotFound(t, err, "resend accepted invitation")
	_, err = h.svc.ResendInvitation(ctx, account(ownerID), testOrg, "unknown")
	accesstest.AssertNotFound(t, err, "resend unknown invitation")
}

func TestRevokeInvitation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.svc.RevokeInvitation(ctx, account(ownerID), testOrg, pendingNew); err != nil {
		t.Fatal(err)
	}
	accesstest.AssertNotFound(t, h.svc.RevokeInvitation(ctx, account(ownerID), testOrg, "unknown"), "revoke unknown invitation")
}

func TestGetInvitation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, tok := range []string{"unknown", "tok-" + acceptedInvite} {
		accesstest.AssertNotFound(t, accesstest.Err(h.svc.GetInvitation(ctx, tok, 0)), tok)
	}

	view, err := h.svc.GetInvitation(ctx, "tok-"+pendingExisting, inviteeID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "pending" || !view.AccountExists || !view.AuthenticatedAsInvitee || view.Organization.ID != testOrg || view.Email != "invitee@example.com" {
		t.Fatalf("view = %+v", view)
	}
	view, err = h.svc.GetInvitation(ctx, "tok-"+pendingExisting, 0)
	if err != nil || view.AuthenticatedAsInvitee {
		t.Fatalf("anonymous view = %+v, %v", view, err)
	}
	view, err = h.svc.GetInvitation(ctx, "tok-"+expiredInvite, 0)
	if err != nil || view.Status != "expired" || view.AccountExists {
		t.Fatalf("expired view = %+v, %v", view, err)
	}
}

func TestAcceptInvitationRejectsBeforeReadingInput(t *testing.T) {
	h := newHarness(t)
	cases := map[string]error{
		"unknown":               orgs.ErrNotFound,
		"tok-" + acceptedInvite: orgs.ErrNotFound,
		"tok-" + expiredInvite:  orgs.ErrInvitationExpired,
	}
	for tok, want := range cases {
		read := false
		_, err := h.svc.AcceptInvitation(context.Background(), tok, 0, func() (orgs.AcceptInvitationInput, error) { read = true; return noInput() })
		if !errors.Is(err, want) || read {
			t.Fatalf("%s: err = %v read = %v, want %v", tok, err, read, want)
		}
	}
	readErr := errors.New("bad body")
	_, err := h.svc.AcceptInvitation(context.Background(), "tok-"+pendingNew, 0, func() (orgs.AcceptInvitationInput, error) {
		return orgs.AcceptInvitationInput{}, readErr
	})
	if err != readErr {
		t.Fatalf("read error = %v", err)
	}
	if h.invites.writes() != 0 {
		t.Fatal("a rejected acceptance changed the store")
	}
}

func TestAcceptInvitationExistingAccount(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		token  string
		viewer int64
		want   error
	}{
		{"anonymous", "tok-" + pendingExisting, 0, orgs.ErrAuthenticationRequired},
		{"other account", "tok-" + pendingExisting, readerID, orgs.ErrForbidden},
		{"inactive invitee", "tok-" + inactiveInvite, inactiveID, orgs.ErrForbidden},
	}
	for _, tc := range cases {
		h := newHarness(t)
		_, err := h.svc.AcceptInvitation(ctx, tc.token, tc.viewer, noInput)
		if !errors.Is(err, tc.want) || h.invites.writes() != 0 {
			t.Fatalf("%s: err = %v writes = %d", tc.name, err, h.invites.writes())
		}
	}

	h := newHarness(t)
	accepted, err := h.svc.AcceptInvitation(ctx, "tok-"+pendingExisting, inviteeID, noInput)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Created || accepted.Account.ID != inviteeID || accepted.Account.Email != "invitee@example.com" || accepted.Organization.ID != testOrg {
		t.Fatalf("accepted = %+v", accepted)
	}
	if len(h.invites.accepted) != 1 || h.invites.accepted[0] != (acceptCall{invitationID: pendingExisting, accountID: inviteeID}) {
		t.Fatalf("accept calls = %+v", h.invites.accepted)
	}
	if len(h.cache.principals) != 1 || h.cache.principals[0] != [2]int64{testOrg, inviteeID} {
		t.Fatalf("principals = %v", h.cache.principals)
	}
	if len(h.events) != 1 || h.events[0].Action != orgs.ActionInvitationAccepted || h.events[0].ResourceID != pendingExisting || *h.events[0].Actor.SubjectID != inviteeID {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestAcceptInvitationCreatesAccount(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, err := h.svc.AcceptInvitation(ctx, "tok-"+pendingNew, readerID, input("New", "securepass99"))
	if !errors.Is(err, orgs.ErrSignOutRequired) {
		t.Fatalf("signed-in caller: %v", err)
	}
	_, err = h.svc.AcceptInvitation(ctx, "tok-"+pendingNew, 0, input(" ", "short"))
	var v *orgs.ValidationError
	if !errors.As(err, &v) || v.Fields["name"] != "Name is required." || v.Fields["password"] != "Password must be at least 8 characters." {
		t.Fatalf("validation = %v", err)
	}
	if h.invites.writes() != 0 {
		t.Fatal("a rejected acceptance changed the store")
	}

	accepted, err := h.svc.AcceptInvitation(ctx, "tok-"+pendingNew, 0, input(" New Person ", "securepass99"))
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.Created || accepted.Account.ID != 99 {
		t.Fatalf("accepted = %+v", accepted)
	}
	created := h.invites.accepted[0].newAccount
	if created == nil || created.Email != "new@example.com" || created.Name != "New Person" || created.PasswordHash == "" || created.PasswordHash == "securepass99" {
		t.Fatalf("new account = %+v", created)
	}
	if len(h.cache.principals) != 1 || h.cache.principals[0] != [2]int64{testOrg, 99} {
		t.Fatalf("principals = %v", h.cache.principals)
	}
	if len(h.events) != 1 || *h.events[0].Actor.SubjectID != 99 {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestAcceptInvitationStoreConflicts(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.invites.acceptErr = orgs.ErrInvitationUnavailable
	_, err := h.svc.AcceptInvitation(ctx, "tok-"+pendingExisting, inviteeID, noInput)
	if !errors.Is(err, orgs.ErrInvitationUnavailable) {
		t.Fatalf("unavailable: %v", err)
	}
	h.invites.acceptErr = errors.New("UNIQUE constraint failed: accounts.email")
	_, err = h.svc.AcceptInvitation(ctx, "tok-"+pendingNew, 0, input("New", "securepass99"))
	if !errors.Is(err, orgs.ErrInvitationAccountExists) {
		t.Fatalf("account exists: %v", err)
	}
	if len(h.cache.principals) != 0 || len(h.events) != 0 {
		t.Fatal("a failed acceptance invalidated policy or audited")
	}
}
