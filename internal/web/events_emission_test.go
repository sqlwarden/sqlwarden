package web

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/events"
)

type testCaptureSink struct {
	mu     sync.Mutex
	events []events.Event
}

func (s *testCaptureSink) HandleEvent(_ context.Context, ev events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *testCaptureSink) find(action, outcome string) (events.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		if ev.Action == action && ev.Outcome == outcome {
			return ev, true
		}
	}
	return events.Event{}, false
}

func TestLoginEmitsDomainEvents(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	sink := &testCaptureSink{}
	app.eventBus.Subscribe(sink)

	alice := testUsers["alice"]
	email := uniqueEmail(t, "login-events")
	account, err := app.db.InsertAccount(context.Background(), email, "Alice", &alice.hashedPassword)
	if err != nil {
		t.Fatal(err)
	}

	res := send(t, newTestRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": alice.password,
	}), app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", res.StatusCode)
	}
	ev, ok := sink.find("auth.login", "success")
	if !ok {
		t.Fatal("expected auth.login success event")
	}
	if ev.ActorID != account.ID {
		t.Fatalf("ActorID = %d, want %d", ev.ActorID, account.ID)
	}

	res = send(t, newTestRequest(t, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email":    email,
		"password": "wrong-password-123",
	}), app.routes())
	if res.StatusCode == http.StatusOK {
		t.Fatal("expected login failure")
	}
	if _, ok := sink.find("auth.login", "failure"); !ok {
		t.Fatal("expected auth.login failure event")
	}
}

func TestOrgPolicyChangesEmitDomainEvents(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	sink := &testCaptureSink{}
	app.eventBus.Subscribe(sink)

	_, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "policy-events-owner"), "Policy Events Owner", "Policy Events Org")
	member, _ := seedAccountWithToken(t, app, uniqueEmail(t, "policy-events-member"), "Policy Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	roleID := createRoleForTest(t, app, org.ID, nil, "org", access.PermOrgRead)

	res := send(t, newAuthRequest(t, http.MethodPost,
		"/api/v1/orgs/"+org.Slug+"/policies",
		map[string]any{
			"role_id":      roleID,
			"subject_type": "account",
			"subject_id":   member.ID,
		}, tok), app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("grant status = %d, want 204", res.StatusCode)
	}

	grantEv, ok := sink.find("policy.binding.grant", "success")
	if !ok {
		t.Fatal("expected policy.binding.grant event")
	}
	if grantEv.OrgID != org.ID {
		t.Fatalf("grant OrgID = %d, want %d", grantEv.OrgID, org.ID)
	}
	if grantEv.Metadata["subject_type"] != "account" {
		t.Fatalf("grant Metadata = %v, want subject_type=account", grantEv.Metadata)
	}

	// Find the binding ID via the policies list, then revoke it.
	res = send(t, newAuthRequest(t, http.MethodGet,
		"/api/v1/orgs/"+org.Slug+"/policies", nil, tok), app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list policies status = %d, want 200", res.StatusCode)
	}
	items, _ := res.BodyFields["items"].([]any)
	var bindingID string
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		itemRoleID, _ := m["role_id"].(float64)
		itemBindingID, _ := m["binding_id"].(float64)
		if int64(itemRoleID) == roleID && itemBindingID > 0 {
			bindingID = strconv.FormatInt(int64(itemBindingID), 10)
		}
	}
	if bindingID == "" {
		t.Fatal("could not find granted binding in policy list")
	}

	res = send(t, newAuthRequest(t, http.MethodDelete,
		"/api/v1/orgs/"+org.Slug+"/policies/"+bindingID, nil, tok), app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", res.StatusCode)
	}

	revokeEv, ok := sink.find("policy.binding.revoke", "success")
	if !ok {
		t.Fatal("expected policy.binding.revoke event")
	}
	if revokeEv.OrgID != org.ID || revokeEv.Resource != "role_binding" {
		t.Fatalf("unexpected revoke event: %+v", revokeEv)
	}
}
