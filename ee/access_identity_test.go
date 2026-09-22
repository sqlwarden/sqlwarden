package ee

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/identity"
)

type grantsFunc func(context.Context, int64, int64, string, string, int64, string) ([]access.Grant, error)

func (f grantsFunc) ExplainGrant(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) ([]access.Grant, error) {
	return f(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func enterpriseTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "enterprise.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	if err := edition.Migrate(context.Background(), newEnterprise(true), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnterprisePolicyOrderAndExplicitDenyPrecedence(t *testing.T) {
	db := enterpriseTestDB(t)
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO ee_access_deny_rules (org_id, permission) VALUES (?, ?)
	`, 2, access.PermWsRead); err != nil {
		t.Fatal(err)
	}
	core := &allowingPolicy{}
	policy := newEnterprise(true).PolicyEvaluator(core, edition.Dependencies{
		DB: db,
		Grants: grantsFunc(func(context.Context, int64, int64, string, string, int64, string) ([]access.Grant, error) {
			return nil, nil
		}),
	})

	if got := policyChainLayers(policy); !slices.Equal(got, policyLayerOrder) {
		t.Fatalf("layers = %v, want %v", got, policyLayerOrder)
	}
	if policy.Can(context.Background(), 1, 2, "org", "workspace", 3, access.PermWsRead) {
		t.Fatal("explicit deny must override the core grant")
	}
}

func TestEnterpriseBindingExpiryRequiresOneLiveGrant(t *testing.T) {
	db := enterpriseTestDB(t)
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	policy := newEnterprise(true).PolicyEvaluator(&allowingPolicy{}, edition.Dependencies{
		DB: db, Now: func() time.Time { return now },
		Grants: grantsFunc(func(context.Context, int64, int64, string, string, int64, string) ([]access.Grant, error) {
			return []access.Grant{{BindingID: 1, ExpiresAt: &expired}}, nil
		}),
	})
	if policy.Can(context.Background(), 1, 2, "org", "workspace", 3, access.PermWsRead) {
		t.Fatal("an expired binding granted access")
	}
}

func TestEnterpriseConditionalAndJITAccessFailClosed(t *testing.T) {
	db := enterpriseTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO ee_conditional_access_rules (org_id, permission, required_auth_method) VALUES (?, ?, ?)
	`, 2, access.PermWsWrite, string(identity.AuthenticationOIDC)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO ee_jit_access_policies (org_id, resource_type, resource_id, permission) VALUES (?, ?, ?, ?)
	`, 2, "workspace", 3, access.PermWsRead); err != nil {
		t.Fatal(err)
	}
	policy := newEnterprise(true).PolicyEvaluator(&allowingPolicy{}, edition.Dependencies{
		DB: db,
		Grants: grantsFunc(func(context.Context, int64, int64, string, string, int64, string) ([]access.Grant, error) {
			return nil, nil
		}),
	})

	if policy.Can(ctx, 1, 2, "org", "workspace", 3, access.PermWsWrite) {
		t.Fatal("conditional access passed without authentication attributes")
	}
	oidcCtx := access.WithAttributes(ctx, access.Attributes{access.AttributeAuthMethod: string(identity.AuthenticationOIDC)})
	if !policy.Can(oidcCtx, 1, 2, "org", "workspace", 3, access.PermWsWrite) {
		t.Fatal("conditional access rejected the required authentication method")
	}
	if policy.Can(ctx, 1, 2, "org", "workspace", 3, access.PermWsRead) {
		t.Fatal("JIT policy passed without an activation")
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO ee_jit_activations (org_id, account_id, resource_type, resource_id, permission, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, 2, 1, "workspace", 3, access.PermWsRead, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !policy.Can(ctx, 1, 2, "org", "workspace", 3, access.PermWsRead) {
		t.Fatal("JIT policy rejected an active grant")
	}
}

func TestEnterpriseFederationMapsOnlyProvisionedDirectorySubjects(t *testing.T) {
	db := enterpriseTestDB(t)
	account, err := db.InsertAccount(context.Background(), "federated@example.com", "Federated User", nil)
	if err != nil {
		t.Fatal(err)
	}
	provisioner := NewDirectoryProvisioner(db)
	if err := provisioner.Provision(context.Background(), DirectoryMapping{
		Provider: "corp-oidc", ExternalID: "subject-42", Email: account.Email, AccountID: account.ID, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	verifier := identity.FederationVerifierFunc(func(context.Context, identity.AuthenticationRequest) (identity.DirectorySubject, error) {
		return identity.DirectorySubject{Provider: "corp-oidc", ExternalID: "subject-42", Email: account.Email, Name: account.Name}, nil
	})
	core := identity.ProviderFunc(func(context.Context, identity.AuthenticationRequest) (identity.Subject, error) {
		return identity.Subject{}, identity.ErrUnsupportedMethod
	})
	provider := newEnterprise(true).IdentityProvider(core, edition.Dependencies{DB: db, Federation: verifier})
	subject, err := provider.Authenticate(context.Background(), identity.AuthenticationRequest{Method: identity.AuthenticationOIDC, Secret: "verified-by-hook"})
	if err != nil {
		t.Fatal(err)
	}
	if subject.AccountID != account.ID {
		t.Fatalf("account id = %d, want %d", subject.AccountID, account.ID)
	}

	if err := provisioner.Deprovision(context.Background(), "corp-oidc", "subject-42"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Authenticate(context.Background(), identity.AuthenticationRequest{Method: identity.AuthenticationOIDC}); err != identity.ErrInvalidCredentials {
		t.Fatalf("deprovisioned authentication error = %v, want ErrInvalidCredentials", err)
	}
}
