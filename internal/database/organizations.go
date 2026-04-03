package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type Organization struct {
	bun.BaseModel `bun:"table:organizations"`

	ID        int64     `bun:",pk,autoincrement" json:"id"`
	Slug      string    `bun:",notnull,unique"   json:"slug"`
	Name      string    `bun:",notnull"          json:"name"`
	CreatedAt time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"          json:"updated_at"`
}

type OrgIDPConfig struct {
	bun.BaseModel `bun:"table:org_idp_configs"`

	ID          string    `bun:",pk"             json:"id"`
	OrgID       int64     `bun:",notnull,unique" json:"org_id"`
	Provider    string    `bun:",notnull"        json:"provider"`
	DisplayName string    `bun:",notnull"        json:"display_name"`
	Config      string    `bun:",notnull"        json:"-"`
	SSORequired bool      `bun:",notnull"        json:"sso_required"`
	IsActive    bool      `bun:",notnull,default:true" json:"is_active"`
	CreatedAt   time.Time `bun:",notnull"        json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"        json:"updated_at"`
}

type OrgMember struct {
	bun.BaseModel `bun:"table:org_members"`

	OrgID     int64     `bun:",pk" json:"org_id"`
	AccountID int64     `bun:",pk" json:"account_id"`
	JoinedAt  time.Time `bun:",notnull" json:"joined_at"`
}

func (db *DB) InsertOrg(ctx context.Context, slug, name string) (Organization, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	org := Organization{
		Slug:      slug,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := db.NewInsert().Model(&org).Returning("id").Exec(ctx)
	if err != nil {
		return Organization{}, err
	}
	return org, nil
}

func (db *DB) GetOrg(ctx context.Context, id int64) (Organization, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var org Organization
	err := db.NewSelect().Model(&org).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, false, nil
	}
	if err != nil {
		return Organization{}, false, err
	}
	return org, true, nil
}

func (db *DB) GetOrgBySlug(ctx context.Context, slug string) (Organization, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var org Organization
	err := db.NewSelect().Model(&org).Where("slug = ?", slug).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, false, nil
	}
	if err != nil {
		return Organization{}, false, err
	}
	return org, true, nil
}

func (db *DB) AddOrgMember(ctx context.Context, orgID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	member := OrgMember{OrgID: orgID, AccountID: accountID, JoinedAt: time.Now()}
	_, err := db.NewInsert().Model(&member).On("CONFLICT DO NOTHING").Exec(ctx)
	return err
}

func (db *DB) RemoveOrgMember(ctx context.Context, orgID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*OrgMember)(nil)).
		Where("org_id = ? AND account_id = ?", orgID, accountID).Exec(ctx)
	return err
}

func (db *DB) GetOrgMembers(ctx context.Context, orgID int64) ([]OrgMember, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var members []OrgMember
	err := db.NewSelect().Model(&members).Where("org_id = ?", orgID).Scan(ctx)
	return members, err
}

func (db *DB) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*OrgMember)(nil)).
		Where("org_id = ? AND account_id = ?", orgID, accountID).Exists(ctx)
}

func (db *DB) GetAccountOrgs(ctx context.Context, accountID int64) ([]Organization, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var orgs []Organization
	err := db.NewSelect().Model(&orgs).
		Join("JOIN org_members AS om ON om.org_id = organization.id").
		Where("om.account_id = ?", accountID).
		Scan(ctx)
	return orgs, err
}

// DeleteAccountRoleBindings removes account-scoped role bindings for a specific resource.
func (db *DB) DeleteAccountRoleBindings(ctx context.Context, orgID, accountID int64, resourceType string, resourceID int64, roleIDs []int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if len(roleIDs) == 0 {
		return nil
	}

	_, err := db.NewDelete().
		TableExpr("role_bindings").
		Where("org_id = ? AND subject_type = 'account' AND subject_id = ? AND resource_type = ? AND resource_id = ?",
			orgID, accountID, resourceType, resourceID).
		Where("role_id IN (?)", bun.List(roleIDs)).
		Exec(ctx)
	return err
}

func (db *DB) UpsertOrgIDPConfig(ctx context.Context, orgID int64, provider, displayName, encryptedConfig string, ssoRequired bool) (OrgIDPConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	config := OrgIDPConfig{
		ID:          newID(),
		OrgID:       orgID,
		Provider:    provider,
		DisplayName: displayName,
		Config:      encryptedConfig,
		SSORequired: ssoRequired,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.NewInsert().Model(&config).
		On("CONFLICT (org_id) DO UPDATE").
		Set("provider = EXCLUDED.provider").
		Set("display_name = EXCLUDED.display_name").
		Set("config = EXCLUDED.config").
		Set("sso_required = EXCLUDED.sso_required").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return OrgIDPConfig{}, err
	}
	return config, nil
}

func (db *DB) GetOrgIDPConfig(ctx context.Context, orgID int64) (OrgIDPConfig, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var config OrgIDPConfig
	err := db.NewSelect().Model(&config).Where("org_id = ?", orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgIDPConfig{}, false, nil
	}
	if err != nil {
		return OrgIDPConfig{}, false, err
	}
	return config, true, nil
}
