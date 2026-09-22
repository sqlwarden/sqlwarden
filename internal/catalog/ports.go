package catalog

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/settings"
)

// Grants is the authorization cache contract the catalog invalidates through.
// Creating, deleting, or re-parenting a resource changes what the enforcer
// would decide, so every use case that does so tells the enforcer to forget
// what it cached. *access.Enforcer satisfies it.
type Grants interface {
	InvalidateOrgPolicy(orgID int64)
	InvalidatePrincipals(orgID, accountID int64)
	InvalidateAncestry(resourceType string, resourceID int64)
}

// Sessions is the live target-database session contract. The catalog counts
// sessions before a change that would invalidate them and drops the ones a
// completed change has made unusable. *connection.Manager satisfies it.
type Sessions interface {
	CountForConnection(connectionID string) int
	RemoveForConnection(connectionID string) int
	RemoveForOrgAccount(orgID, accountID string) int
}

// Sealer encrypts and decrypts connection secrets. The catalog never persists
// or returns a DSN that has not been through it. *encrypt.Keyring satisfies it.
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// InstanceSettingsReader reads the instance settings that gate which target
// databases may be registered. *settings.Service satisfies it.
type InstanceSettingsReader interface {
	Instance(ctx context.Context) (database.InstanceSettings, error)
}

var (
	_ Grants                 = (*access.Enforcer)(nil)
	_ Sessions               = (*connection.Manager)(nil)
	_ Sealer                 = (*encrypt.Keyring)(nil)
	_ InstanceSettingsReader = (*settings.Service)(nil)
)
