package editiontest

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/identity"
)

// Run verifies delegation, denial preservation, module validation, and
// entitlement isolation for an Edition implementation. Decorators are composed
// with deps, so an edition under test receives the same dependencies the
// composition root supplies.
func Run(t *testing.T, candidate edition.Edition, cfg config.Config, deps edition.Dependencies) {
	t.Helper()
	if err := edition.Validate(candidate, cfg); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if candidate.Name() != cfg.Edition.Name {
		t.Fatalf("Name = %q, want %q", candidate.Name(), cfg.Edition.Name)
	}

	identityCalled := false
	coreIdentity := identity.ProviderFunc(func(context.Context, identity.AuthenticationRequest) (identity.Subject, error) {
		identityCalled = true
		return identity.Subject{AccountID: 42}, nil
	})
	subject, err := candidate.IdentityProvider(coreIdentity, deps).Authenticate(context.Background(), identity.AuthenticationRequest{
		Method: identity.AuthenticationPassword, Identifier: "contract@example.com", Secret: "not-logged",
	})
	if err != nil || !identityCalled || subject.AccountID != 42 {
		t.Fatalf("identity delegation: subject=%+v called=%t err=%v", subject, identityCalled, err)
	}

	corePolicy := &policyEvaluator{allow: false}
	if candidate.PolicyEvaluator(corePolicy, deps).Can(context.Background(), 1, 2, "org", "workspace", 3, "workspace:read") {
		t.Fatal("edition policy granted a permission denied by core")
	}
	if corePolicy.calls != 1 {
		t.Fatalf("core policy calls = %d, want 1", corePolicy.calls)
	}

	auditCalled := false
	coreAudit := audit.WriterFunc(func(context.Context, audit.Event) error {
		auditCalled = true
		return nil
	})
	if err := candidate.AuditWriter(coreAudit, deps).Write(context.Background(), audit.Event{Action: "contract", Outcome: audit.OutcomeSuccess}); err != nil {
		t.Fatal(err)
	}
	if !auditCalled {
		t.Fatal("edition audit writer did not delegate to core")
	}

	entitlements := candidate.Entitlements()
	for _, module := range candidate.Modules() {
		for capability, enabled := range module.Capabilities() {
			if enabled && !entitlements.Enabled(capability) {
				t.Errorf("module %q capability %q missing from entitlements", module.Name(), capability)
			}
		}
	}
	mutatedCapability := ""
	for capability := range entitlements {
		mutatedCapability = capability
		delete(entitlements, capability)
		break
	}
	if mutatedCapability != "" && !candidate.Entitlements().Enabled(mutatedCapability) {
		t.Fatal("mutating returned entitlements changed edition state")
	}
}

type policyEvaluator struct {
	allow bool
	calls int
}

func (p *policyEvaluator) Can(context.Context, int64, int64, string, string, int64, string) bool {
	p.calls++
	return p.allow
}

func (p *policyEvaluator) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	if p.allow {
		return []string{"workspace:read"}, nil
	}
	return nil, nil
}

var _ access.PolicyEvaluator = (*policyEvaluator)(nil)
