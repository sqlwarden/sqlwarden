package ee

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
)

type federationIdentityProvider struct {
	core     identity.Provider
	verifier identity.FederationVerifier
	store    *store
}

func (p federationIdentityProvider) Authenticate(ctx context.Context, request identity.AuthenticationRequest) (identity.Subject, error) {
	if request.Method == identity.AuthenticationPassword {
		return p.core.Authenticate(ctx, request)
	}
	if request.Method != identity.AuthenticationOIDC && request.Method != identity.AuthenticationSAML {
		return identity.Subject{}, identity.ErrUnsupportedMethod
	}
	if p.verifier == nil || p.store == nil {
		return identity.Subject{}, identity.ErrUnsupportedMethod
	}

	directorySubject, err := p.verifier.Verify(ctx, request)
	if err != nil {
		return identity.Subject{}, identity.ErrInvalidCredentials
	}
	provider := strings.TrimSpace(directorySubject.Provider)
	externalID := strings.TrimSpace(directorySubject.ExternalID)
	email := strings.TrimSpace(directorySubject.Email)
	if provider == "" || (externalID == "" && email == "") {
		return identity.Subject{}, identity.ErrInvalidCredentials
	}

	var mapped directoryIdentity
	var found bool
	if externalID != "" {
		mapped, found, err = p.store.directoryIdentityByExternalID(ctx, provider, externalID)
	} else {
		mapped, found, err = p.store.directoryIdentityByEmail(ctx, provider, email)
	}
	if err != nil {
		return identity.Subject{}, err
	}
	if !found || !mapped.IsActive {
		return identity.Subject{}, identity.ErrInvalidCredentials
	}
	return identity.Subject{
		AccountID:  mapped.AccountID,
		Email:      mapped.Email,
		Name:       directorySubject.Name,
		Attributes: directorySubject.Attributes,
	}, nil
}

// DirectoryProvisioner is the Enterprise directory-mapping hook used by SCIM
// and administrative provisioning transports. It changes only Enterprise
// identity mappings; account lifecycle remains a core identity use case.
type DirectoryProvisioner struct {
	store *store
	now   func() time.Time
}

// NewDirectoryProvisioner returns a provisioning adapter over the metadata
// database supplied to the edition composition.
func NewDirectoryProvisioner(db *database.DB) *DirectoryProvisioner {
	return &DirectoryProvisioner{store: newStore(db), now: time.Now}
}

// DirectoryMapping identifies a verified directory subject and its core
// account mapping.
type DirectoryMapping struct {
	Provider   string
	ExternalID string
	Email      string
	AccountID  int64
	Active     bool
}

// Provision creates or refreshes a directory mapping idempotently.
func (p *DirectoryProvisioner) Provision(ctx context.Context, mapping DirectoryMapping) error {
	if p == nil || p.store == nil {
		return errStoreUnavailable
	}
	if strings.TrimSpace(mapping.Provider) == "" || strings.TrimSpace(mapping.ExternalID) == "" || mapping.AccountID <= 0 {
		return errors.New("invalid directory mapping")
	}
	return p.store.linkDirectoryIdentity(ctx, directoryIdentity{
		Provider: mapping.Provider, ExternalID: mapping.ExternalID, Email: mapping.Email,
		AccountID: mapping.AccountID, IsActive: mapping.Active,
	}, p.now())
}

// Deprovision disables a directory mapping so it can no longer authenticate.
func (p *DirectoryProvisioner) Deprovision(ctx context.Context, provider, externalID string) error {
	if p == nil || p.store == nil {
		return errStoreUnavailable
	}
	return p.store.deactivateDirectoryIdentity(ctx, provider, externalID, p.now())
}

var _ identity.Provider = federationIdentityProvider{}
