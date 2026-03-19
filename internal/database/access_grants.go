package database

import (
	"context"
	"time"
)

type AccessGrant struct {
	ID        string     `bun:",pk"       json:"id"`
	TenantID  string     `bun:",notnull"  json:"tenant_id"`
	Subject   string     `bun:",notnull"  json:"subject"`
	Object    string     `bun:",notnull"  json:"object"`
	Action    string     `bun:",notnull"  json:"action"`
	GrantedBy string     `bun:",notnull"  json:"granted_by"`
	ExpiresAt *time.Time `bun:",nullzero" json:"expires_at,omitempty"`
	CreatedAt time.Time  `bun:",notnull"  json:"created_at"`
}

func (db *DB) InsertAccessGrant(tenantID, subject, object, action, grantedBy string, expiresAt *time.Time) (AccessGrant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	grant := AccessGrant{
		ID:        newID(),
		TenantID:  tenantID,
		Subject:   subject,
		Object:    object,
		Action:    action,
		GrantedBy: grantedBy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&grant).
		Exec(ctx)
	if err != nil {
		return AccessGrant{}, err
	}

	return grant, nil
}

func (db *DB) GetAccessGrantsByObject(object string) ([]AccessGrant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var grants []AccessGrant
	err := db.NewSelect().
		Model(&grants).
		Where("object = ?", object).
		Scan(ctx)

	return grants, err
}

func (db *DB) DeleteAccessGrant(subject, object string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*AccessGrant)(nil)).
		Where("subject = ? AND object = ?", subject, object).
		Exec(ctx)

	return err
}

func (db *DB) GetExpiredAccessGrants() ([]AccessGrant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var grants []AccessGrant
	err := db.NewSelect().
		Model(&grants).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Scan(ctx)

	return grants, err
}
