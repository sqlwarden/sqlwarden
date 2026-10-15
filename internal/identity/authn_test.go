package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
)

type fakeAuthenticator struct {
	claim bool
	err   error
	id    int64
}

func (f fakeAuthenticator) Authenticate(context.Context, Presented) (Authenticated, bool, error) {
	if !f.claim {
		return Authenticated{}, false, nil
	}
	if f.err != nil {
		return Authenticated{}, true, f.err
	}
	return Authenticated{Principal: access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: f.id}}}, true, nil
}

type denyPolicy struct{}

func (denyPolicy) Evaluate(context.Context, access.Principal) access.Decision {
	return access.Deny("blocked_by_test")
}

type emptyDenyPolicy struct{}

func (emptyDenyPolicy) Evaluate(context.Context, access.Principal) access.Decision {
	return access.Decision{Effect: access.EffectDeny}
}

type postureFunc func(*access.Signals)

func (f postureFunc) Collect(_ context.Context, _ Presented, s *access.Signals) error {
	f(s)
	return nil
}

func TestChainFirstClaimWins(t *testing.T) {
	c := Chain{Authenticators: []Authenticator{fakeAuthenticator{}, fakeAuthenticator{claim: true, id: 7}, fakeAuthenticator{claim: true, id: 9}}}
	got, err := c.Authenticate(context.Background(), Presented{})
	if err != nil || got.Principal.Subject.ID != 7 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestChainUnclaimedIsCredentialError(t *testing.T) {
	_, err := Chain{Authenticators: []Authenticator{fakeAuthenticator{}}}.Authenticate(context.Background(), Presented{})
	var ce *CredentialError
	if !errors.As(err, &ce) || ce.Reason != "credential_unrecognized" {
		t.Fatalf("err = %v", err)
	}
}

func TestChainPropagatesClaimError(t *testing.T) {
	want := &CredentialError{Reason: "auth_session_revoked"}
	_, err := Chain{Authenticators: []Authenticator{fakeAuthenticator{claim: true, err: want}}}.Authenticate(context.Background(), Presented{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestChainPolicyDenyBecomesCredentialError(t *testing.T) {
	c := Chain{Authenticators: []Authenticator{fakeAuthenticator{claim: true, id: 1}}, Policies: []RequestPolicy{denyPolicy{}}}
	_, err := c.Authenticate(context.Background(), Presented{})
	var ce *CredentialError
	if !errors.As(err, &ce) || ce.Reason != "blocked_by_test" {
		t.Fatalf("err = %v", err)
	}
}

func TestChainPolicyDenyWithoutReasonUsesDefault(t *testing.T) {
	c := Chain{Authenticators: []Authenticator{fakeAuthenticator{claim: true, id: 1}}, Policies: []RequestPolicy{emptyDenyPolicy{}}}
	_, err := c.Authenticate(context.Background(), Presented{})
	var ce *CredentialError
	if !errors.As(err, &ce) || ce.Reason != "request_policy_denied" {
		t.Fatalf("err = %v", err)
	}
}

func TestChainPostureFillsSignals(t *testing.T) {
	c := Chain{
		Authenticators: []Authenticator{fakeAuthenticator{claim: true, id: 1}},
		Posture:        []PostureProvider{postureFunc(func(s *access.Signals) { s.DeviceID = &access.Signal[string]{Value: "dev-1"} })},
	}
	got, err := c.Authenticate(context.Background(), Presented{})
	if err != nil || got.Principal.Request.Signals.DeviceID == nil || got.Principal.Request.Signals.DeviceID.Value != "dev-1" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
