package editiontest

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/identity"
)

// Run verifies composition, core delegation, denial preservation, and
// capability isolation for a concrete Edition implementation.
func Run(t *testing.T, candidate edition.Edition, deps edition.Dependencies) {
	t.Helper()
	composition, err := edition.Compose(context.Background(), candidate, deps)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if composition.Name() != candidate.Name() {
		t.Fatalf("composition name = %q, want %q", composition.Name(), candidate.Name())
	}

	auditCalled := false
	writer := composition.Audit(audit.WriterFunc(func(context.Context, audit.Event) error {
		auditCalled = true
		return nil
	}))
	if err := writer.Write(context.Background(), audit.Event{ID: "contract", Action: "contract", Outcome: audit.OutcomeSuccess}); err != nil {
		t.Fatal(err)
	}
	if !auditCalled {
		t.Fatal("edition audit writer did not delegate to core durability")
	}

	corePolicy := fixedPolicy{allow: false}
	if composition.Policy(corePolicy).Can(context.Background(), 1, 2, "org", "workspace", 3, access.PermWsRead) {
		t.Fatal("edition policy granted a permission denied by core")
	}

	authCalled := false
	coreAuthenticator := authenticatorFunc(func(context.Context, identity.Presented) (identity.Authenticated, bool, error) {
		authCalled = true
		return identity.Authenticated{}, true, nil
	})
	if _, claimed, err := composition.Authenticator(coreAuthenticator).Authenticate(context.Background(), identity.Presented{}); err != nil || !claimed || !authCalled {
		t.Fatalf("identity delegation: called=%t claimed=%t err=%v", authCalled, claimed, err)
	}

	capabilities := composition.Capabilities()
	if len(capabilities.Features) > 0 {
		key := capabilities.Features[0].Key
		state := capabilities.Features[0].State
		capabilities.Features[0].State = edition.StateUpgrade
		if got := composition.Capabilities().Features[0]; got.Key != key || got.State != state {
			t.Fatal("mutating returned capabilities changed edition state")
		}
	}
}

type fixedPolicy struct{ allow bool }

func (p fixedPolicy) Can(context.Context, int64, int64, string, string, int64, string) bool {
	return p.allow
}
func (p fixedPolicy) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

type authenticatorFunc func(context.Context, identity.Presented) (identity.Authenticated, bool, error)

func (f authenticatorFunc) Authenticate(ctx context.Context, presented identity.Presented) (identity.Authenticated, bool, error) {
	return f(ctx, presented)
}
