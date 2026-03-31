package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type Tenant struct {
	ID        string    `bun:",pk"              json:"id"`
	Slug      string    `bun:",notnull,unique"  json:"slug"`
	Name      string    `bun:",notnull"         json:"name"`
	CreatedAt time.Time `bun:",notnull"         json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"         json:"updated_at"`
}

type TenantIDPConfig struct {
	bun.BaseModel `bun:"table:tenant_idp_configs"`

	ID          string    `bun:",pk"                      json:"id"`
	TenantID    string    `bun:",notnull,unique"          json:"tenant_id"`
	Provider    string    `bun:",notnull"                 json:"provider"`
	DisplayName string    `bun:",notnull"                 json:"display_name"`
	Config      string    `bun:",notnull"                 json:"-"`
	IsActive    bool      `bun:",notnull,default:true"    json:"is_active"`
	CreatedAt   time.Time `bun:",notnull"                 json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"                 json:"updated_at"`
}

type TenantMember struct {
	TenantID  string    `bun:",pk" json:"tenant_id"`
	AccountID int64     `bun:",pk" json:"account_id"`
	Role      string    `bun:",notnull,default:'member'" json:"role"`
	CreatedAt time.Time `bun:",notnull" json:"created_at"`
}

func (db *DB) InsertTenant(slug, name string) (Tenant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	tenant := Tenant{
		ID:        newID(),
		Slug:      slug,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&tenant).
		Exec(ctx)
	if err != nil {
		return Tenant{}, err
	}

	return tenant, nil
}

func (db *DB) GetTenant(id string) (Tenant, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var tenant Tenant
	err := db.NewSelect().
		Model(&tenant).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, nil
	}
	if err != nil {
		return Tenant{}, false, err
	}

	return tenant, true, nil
}

func (db *DB) GetTenantBySlug(slug string) (Tenant, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var tenant Tenant
	err := db.NewSelect().
		Model(&tenant).
		Where("slug = ?", slug).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, false, nil
	}
	if err != nil {
		return Tenant{}, false, err
	}

	return tenant, true, nil
}

func (db *DB) AddTenantMember(tenantID string, accountID int64, role string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	member := TenantMember{
		TenantID:  tenantID,
		AccountID: accountID,
		Role:      role,
		CreatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&member).
		Exec(ctx)

	return err
}

func (db *DB) RemoveTenantMember(tenantID string, accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*TenantMember)(nil)).
		Where("tenant_id = ? AND account_id = ?", tenantID, accountID).
		Exec(ctx)

	return err
}

func (db *DB) UpdateTenantMemberRole(tenantID string, accountID int64, role string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*TenantMember)(nil)).
		Set("role = ?", role).
		Where("tenant_id = ? AND account_id = ?", tenantID, accountID).
		Exec(ctx)

	return err
}

func (db *DB) GetTenantMembers(tenantID string) ([]TenantMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var members []TenantMember
	err := db.NewSelect().
		Model(&members).
		Where("tenant_id = ?", tenantID).
		Scan(ctx)

	return members, err
}

func (db *DB) GetAccountTenants(accountID int64) ([]Tenant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var tenants []Tenant
	err := db.NewSelect().
		Model(&tenants).
		Join("JOIN tenant_members AS tm ON tm.tenant_id = tenant.id").
		Where("tm.account_id = ?", accountID).
		Scan(ctx)

	return tenants, err
}

func (db *DB) IsTenantMember(tenantID string, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	exists, err := db.NewSelect().
		Model((*TenantMember)(nil)).
		Where("tenant_id = ? AND account_id = ?", tenantID, accountID).
		Exists(ctx)

	return exists, err
}

func (db *DB) CountTenantOwners(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	count, err := db.NewSelect().
		Model((*TenantMember)(nil)).
		Where("tenant_id = ? AND role = ?", tenantID, "owner").
		Count(ctx)

	return count, err
}

func (db *DB) UpsertTenantIDPConfig(tenantID, provider, displayName, encryptedConfig string) (TenantIDPConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	now := time.Now()
	config := TenantIDPConfig{
		ID:          newID(),
		TenantID:    tenantID,
		Provider:    provider,
		DisplayName: displayName,
		Config:      encryptedConfig,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := db.NewInsert().
		Model(&config).
		On("CONFLICT (tenant_id) DO UPDATE").
		Set("provider = EXCLUDED.provider").
		Set("display_name = EXCLUDED.display_name").
		Set("config = EXCLUDED.config").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return TenantIDPConfig{}, err
	}

	return config, nil
}

func (db *DB) GetTenantIDPConfig(tenantID string) (TenantIDPConfig, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var config TenantIDPConfig
	err := db.NewSelect().
		Model(&config).
		Where("tenant_id = ?", tenantID).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return TenantIDPConfig{}, false, nil
	}
	if err != nil {
		return TenantIDPConfig{}, false, err
	}

	return config, true, nil
}

func (db *DB) DeleteTenantIDPConfig(tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*TenantIDPConfig)(nil)).
		Where("tenant_id = ?", tenantID).
		Exec(ctx)

	return err
}
