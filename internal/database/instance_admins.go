package database

import (
	"context"
	"time"

	"github.com/uptrace/bun"
)

type InstanceAdmin struct {
	bun.BaseModel `bun:"table:instance_admins"`

	AccountID int64     `bun:"account_id,pk"    json:"account_id"`
	CreatedAt time.Time `bun:",notnull"         json:"created_at"`
	Account   *Account  `bun:"rel:belongs-to,join:account_id=id" json:"account,omitempty"`
}

func (db *DB) IsInstanceAdmin(accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	exists, err := db.NewSelect().Model((*InstanceAdmin)(nil)).
		Where("account_id = ?", accountID).
		Exists(ctx)
	return exists, err
}

func (db *DB) InsertInstanceAdmin(accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	admin := &InstanceAdmin{AccountID: accountID, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(admin).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

func (db *DB) RemoveInstanceAdmin(accountID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*InstanceAdmin)(nil)).
		Where("account_id = ?", accountID).
		Exec(ctx)
	return err
}

func (db *DB) ListInstanceAdmins() ([]InstanceAdmin, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var admins []InstanceAdmin
	err := db.NewSelect().Model(&admins).
		Relation("Account").
		OrderExpr("instance_admin.created_at ASC").
		Scan(ctx)
	return admins, err
}

func (db *DB) CountInstanceAdmins() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*InstanceAdmin)(nil)).Count(ctx)
}

func (db *DB) HasAnyInstanceAdmin() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*InstanceAdmin)(nil)).Exists(ctx)
}
