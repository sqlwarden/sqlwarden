package ee

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/uptrace/bun"
)

// wildcardResource marks a rule that applies to every resource in an
// organization. Rules stored with an empty resource type and a zero resource id
// match any resource the decision is made about.
const wildcardResource = ""

// store reads Enterprise-owned tables. It never touches core tables: core
// ownership of accounts, roles, and bindings stays in core packages, and an
// Enterprise decorator only adds restrictions on top of a core decision.
type store struct {
	db bun.IDB
}

func newStore(db *database.DB) *store {
	if db == nil {
		return nil
	}
	return &store{db: db.DB}
}

// denied reports whether an explicit deny rule covers this decision. A rule
// with a null account id denies the permission for every member of the
// organization.
func (s *store) denied(ctx context.Context, accountID, orgID int64, resourceType string, resourceID int64, permission string) (bool, error) {
	return s.db.NewSelect().
		TableExpr("ee_access_deny_rules").
		Where("org_id = ?", orgID).
		Where("account_id IS NULL OR account_id = ?", accountID).
		Where("permission = ?", permission).
		Where("resource_type = ? OR (resource_type = ? AND resource_id = ?)", wildcardResource, resourceType, resourceID).
		Exists(ctx)
}

// requiredAuthMethods returns the authentication methods allowed by
// conditional access rules for this permission. An empty result means the
// permission is unconditional.
func (s *store) requiredAuthMethods(ctx context.Context, orgID int64, permission string) ([]string, error) {
	var methods []string
	err := s.db.NewSelect().
		TableExpr("ee_conditional_access_rules").
		ColumnExpr("required_auth_method").
		Where("org_id = ?", orgID).
		Where("permission = ?", permission).
		Scan(ctx, &methods)
	if err != nil {
		return nil, err
	}
	return methods, nil
}

// jitRequired reports whether this permission may only be exercised through an
// activated just-in-time grant.
func (s *store) jitRequired(ctx context.Context, orgID int64, resourceType string, resourceID int64, permission string) (bool, error) {
	return s.db.NewSelect().
		TableExpr("ee_jit_access_policies").
		Where("org_id = ?", orgID).
		Where("permission = ?", permission).
		Where("resource_type = ? OR (resource_type = ? AND resource_id = ?)", wildcardResource, resourceType, resourceID).
		Exists(ctx)
}

// jitActive reports whether the account holds an unexpired activation for this
// permission.
func (s *store) jitActive(ctx context.Context, accountID, orgID int64, resourceType string, resourceID int64, permission string, now time.Time) (bool, error) {
	return s.db.NewSelect().
		TableExpr("ee_jit_activations").
		Where("org_id = ?", orgID).
		Where("account_id = ?", accountID).
		Where("permission = ?", permission).
		Where("resource_type = ? OR (resource_type = ? AND resource_id = ?)", wildcardResource, resourceType, resourceID).
		Where("expires_at > ?", now).
		Exists(ctx)
}

// directoryIdentity is a federated identity mapped to a SQLWarden account.
type directoryIdentity struct {
	ID         int64  `bun:"id"`
	Provider   string `bun:"provider"`
	ExternalID string `bun:"external_id"`
	Email      string `bun:"email"`
	AccountID  int64  `bun:"account_id"`
	IsActive   bool   `bun:"is_active"`
}

// directoryIdentityByExternalID resolves a federated assertion subject.
func (s *store) directoryIdentityByExternalID(ctx context.Context, provider, externalID string) (directoryIdentity, bool, error) {
	var identity directoryIdentity
	err := s.db.NewSelect().
		TableExpr("ee_directory_identities").
		ColumnExpr("id, provider, external_id, email, account_id, is_active").
		Where("provider = ?", provider).
		Where("external_id = ?", externalID).
		Limit(1).
		Scan(ctx, &identity)
	return scanned(identity, err)
}

// directoryIdentityByEmail resolves a federated assertion that carries only an
// email address.
func (s *store) directoryIdentityByEmail(ctx context.Context, provider, email string) (directoryIdentity, bool, error) {
	var identity directoryIdentity
	err := s.db.NewSelect().
		TableExpr("ee_directory_identities").
		ColumnExpr("id, provider, external_id, email, account_id, is_active").
		Where("provider = ?", provider).
		Where("email = ?", email).
		Limit(1).
		Scan(ctx, &identity)
	return scanned(identity, err)
}

// linkDirectoryIdentity records or refreshes the mapping between a directory
// subject and a SQLWarden account. It is the provisioning write shared by
// federated login and SCIM.
func (s *store) linkDirectoryIdentity(ctx context.Context, identity directoryIdentity, now time.Time) error {
	result, err := s.db.NewUpdate().
		TableExpr("ee_directory_identities").
		Set("email = ?", identity.Email).
		Set("account_id = ?", identity.AccountID).
		Set("is_active = ?", identity.IsActive).
		Set("updated_at = ?", now).
		Where("provider = ?", identity.Provider).
		Where("external_id = ?", identity.ExternalID).
		Exec(ctx)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected > 0 {
		return nil
	}

	_, err = s.db.NewInsert().
		TableExpr("ee_directory_identities").
		Model(&map[string]any{
			"provider":    identity.Provider,
			"external_id": identity.ExternalID,
			"email":       identity.Email,
			"account_id":  identity.AccountID,
			"is_active":   identity.IsActive,
			"created_at":  now,
			"updated_at":  now,
		}).
		Exec(ctx)
	return err
}

// deactivateDirectoryIdentity marks a directory subject as deprovisioned.
// Account deletion stays a core use case; federated login stops immediately.
func (s *store) deactivateDirectoryIdentity(ctx context.Context, provider, externalID string, now time.Time) error {
	_, err := s.db.NewUpdate().
		TableExpr("ee_directory_identities").
		Set("is_active = ?", false).
		Set("updated_at = ?", now).
		Where("provider = ?", provider).
		Where("external_id = ?", externalID).
		Exec(ctx)
	return err
}

func scanned(identity directoryIdentity, err error) (directoryIdentity, bool, error) {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return directoryIdentity{}, false, nil
		}
		return directoryIdentity{}, false, err
	}
	return identity, true, nil
}
