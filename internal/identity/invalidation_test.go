package identity_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/platform/invalidate"
)

type publication struct {
	topic invalidate.Topic
	key   string
}

type recordingInvalidator struct {
	published []publication
	err       error
}

func (r *recordingInvalidator) Publish(_ context.Context, topic invalidate.Topic, key string) error {
	r.published = append(r.published, publication{topic, key})
	return r.err
}

func (r *recordingInvalidator) Subscribe(invalidate.Topic, func(string)) func() { return func() {} }

func newInvalidationHarness(t *testing.T, inv invalidate.Invalidator) *harness {
	t.Helper()
	h := newHarness(t)
	h.svc = identity.New(identity.Deps{
		Store:       h.store,
		SignIn:      identity.PasswordSignIn(h.store),
		Requirer:    access.NewRequirer(fakeDirectory{store: h.store}, h.policy),
		Tokens:      fixedTTL(15 * time.Minute),
		Secret:      secret,
		Now:         func() time.Time { return h.now },
		Invalidator: inv,
	})
	return h
}

func TestCredentialRevocationsPublish(t *testing.T) {
	ctx := context.Background()
	acct := "account:" + strconv.FormatInt(alice, 10)

	t.Run("password change", func(t *testing.T) {
		rec := &recordingInvalidator{}
		h := newInvalidationHarness(t, rec)
		cur := h.login(t, "alice@example.com", alicePass)
		p := sessionPrincipal(alice, cur.AuthSessionID)
		if err := h.svc.UpdatePassword(ctx, p, identity.UpdatePasswordInput{CurrentPassword: alicePass, NewPassword: "new-password"}); err != nil {
			t.Fatal(err)
		}
		want := []publication{{invalidate.TopicCredentials, acct}}
		if !reflect.DeepEqual(rec.published, want) {
			t.Fatalf("published = %v, want %v", rec.published, want)
		}
	})

	t.Run("logout", func(t *testing.T) {
		rec := &recordingInvalidator{}
		h := newInvalidationHarness(t, rec)
		issued := h.login(t, "alice@example.com", alicePass)
		if _, err := h.svc.Logout(ctx, identity.LogoutInput{RefreshToken: issued.RefreshToken}); err != nil {
			t.Fatal(err)
		}
		want := []publication{{invalidate.TopicCredentials, "session:" + issued.AuthSessionID}}
		if !reflect.DeepEqual(rec.published, want) {
			t.Fatalf("published = %v, want %v", rec.published, want)
		}
	})

	t.Run("admin revokes one session", func(t *testing.T) {
		rec := &recordingInvalidator{}
		h := newInvalidationHarness(t, rec)
		target := h.login(t, "alice@example.com", alicePass)
		if err := h.svc.RevokeInstanceSession(ctx, admin(bob), alice, target.AuthSessionID); err != nil {
			t.Fatal(err)
		}
		want := []publication{{invalidate.TopicCredentials, "session:" + target.AuthSessionID}}
		if !reflect.DeepEqual(rec.published, want) {
			t.Fatalf("published = %v, want %v", rec.published, want)
		}
	})

	t.Run("admin revokes all sessions", func(t *testing.T) {
		rec := &recordingInvalidator{}
		h := newInvalidationHarness(t, rec)
		h.login(t, "alice@example.com", alicePass)
		if err := h.svc.RevokeInstanceSessions(ctx, admin(bob), alice); err != nil {
			t.Fatal(err)
		}
		want := []publication{{invalidate.TopicCredentials, acct}}
		if !reflect.DeepEqual(rec.published, want) {
			t.Fatalf("published = %v, want %v", rec.published, want)
		}
	})

	t.Run("failed revocation publishes nothing", func(t *testing.T) {
		rec := &recordingInvalidator{}
		h := newInvalidationHarness(t, rec)
		if err := h.svc.RevokeInstanceSessions(ctx, admin(bob), 999); !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
		if len(rec.published) != 0 {
			t.Fatalf("published = %v", rec.published)
		}
	})

	t.Run("publish error does not fail the operation", func(t *testing.T) {
		rec := &recordingInvalidator{err: errors.New("bus down")}
		h := newInvalidationHarness(t, rec)
		issued := h.login(t, "alice@example.com", alicePass)
		if _, err := h.svc.Logout(ctx, identity.LogoutInput{RefreshToken: issued.RefreshToken}); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if err := h.svc.RevokeInstanceSessions(ctx, admin(bob), alice); err != nil {
			t.Fatalf("revoke all: %v", err)
		}
		if len(rec.published) != 2 {
			t.Fatalf("published = %v", rec.published)
		}
	})
}
