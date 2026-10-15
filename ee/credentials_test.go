//go:build enterprise

package ee

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/credentials/credentialstest"
	"github.com/sqlwarden/internal/edition"
)

func TestEnterpriseModuleCanRegisterCredentialDecorator(t *testing.T) {
	resolveCalls := 0
	composition, err := edition.Compose(context.Background(), credentialTestEdition{resolveCalls: &resolveCalls}, edition.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if unwired := composition.Unwired(); len(unwired) != 0 {
		t.Fatalf("credential decorator reported as unwired: %v", unwired)
	}

	core := credentialstest.NewReferenceProvider(nil)
	provider := composition.Credentials(core)
	_, err = provider.Resolve(context.Background(), credentials.ConnectionRef{ConnectionID: "missing"})
	if !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("Resolve error = %v, want core ErrNotFound", err)
	}
	if resolveCalls != 1 {
		t.Fatalf("decorator Resolve calls = %d, want 1", resolveCalls)
	}
}

type credentialTestEdition struct{ resolveCalls *int }

func (credentialTestEdition) Name() string               { return edition.EnterpriseName }
func (credentialTestEdition) Licenser() edition.Licenser { return edition.NoopLicenser{} }
func (e credentialTestEdition) Modules() []edition.Module {
	return []edition.Module{credentialTestModule{resolveCalls: e.resolveCalls}}
}

type credentialTestModule struct{ resolveCalls *int }

func (credentialTestModule) Name() string    { return "credentials" }
func (credentialTestModule) Feature() string { return edition.Catalog[0].Key }
func (m credentialTestModule) Register(r *edition.Registrar) error {
	return r.DecorateCredential(func(next credentials.Provider) credentials.Provider {
		return credentialTestProvider{Provider: next, resolveCalls: m.resolveCalls}
	})
}

type credentialTestProvider struct {
	credentials.Provider
	resolveCalls *int
}

func (p credentialTestProvider) Resolve(ctx context.Context, ref credentials.ConnectionRef) (credentials.Credentials, error) {
	(*p.resolveCalls)++
	return p.Provider.Resolve(ctx, ref)
}
