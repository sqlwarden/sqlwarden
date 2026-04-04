package database

import (
	"context"
	"time"

	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

type InstanceAdmin struct {
	bun.BaseModel `bun:"table:instance_admins"`

	AccountID int64     `bun:"account_id,pk"    json:"account_id"`
	CreatedAt time.Time `bun:",notnull"         json:"created_at"`
	Account   *Account  `bun:"rel:belongs-to,join:account_id=id" json:"account,omitempty"`
}

type ListInstanceAdminsParams struct {
	Sort     string
	Order    string
	Page     int
	PageSize int
}

func (db *DB) IsInstanceAdmin(ctx context.Context, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	exists, err := db.NewSelect().Model((*InstanceAdmin)(nil)).
		Where("account_id = ?", accountID).
		Exists(ctx)
	return exists, err
}

func (db *DB) InsertInstanceAdmin(ctx context.Context, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	admin := &InstanceAdmin{AccountID: accountID, CreatedAt: time.Now()}
	_, err := db.NewInsert().Model(admin).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

func (db *DB) RemoveInstanceAdmin(ctx context.Context, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*InstanceAdmin)(nil)).
		Where("account_id = ?", accountID).
		Exec(ctx)
	return err
}

func (db *DB) ListInstanceAdminsPage(ctx context.Context, params ListInstanceAdminsParams) (response.Paginated[InstanceAdmin], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeInstanceAdminListParams(params)

	var admins []InstanceAdmin
	err := db.NewSelect().Model(&admins).
		Relation("Account").
		OrderExpr(instanceAdminOrderExpr(params)).
		Scan(ctx)
	if err != nil {
		return response.Paginated[InstanceAdmin]{}, err
	}
	return response.PaginateItems(admins, params.Page, params.PageSize), nil
}

func (db *DB) CountInstanceAdmins(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*InstanceAdmin)(nil)).Count(ctx)
}

func (db *DB) HasAnyInstanceAdmin(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*InstanceAdmin)(nil)).Exists(ctx)
}

func normalizeInstanceAdminListParams(params ListInstanceAdminsParams) ListInstanceAdminsParams {
	switch params.Sort {
	case "account_id", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "desc":
		params.Order = "desc"
	default:
		params.Order = "asc"
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func instanceAdminOrderExpr(params ListInstanceAdminsParams) string {
	switch params.Sort {
	case "account_id":
		return "instance_admin.account_id " + params.Order + ", instance_admin.created_at " + params.Order
	default:
		return "instance_admin.created_at " + params.Order + ", instance_admin.account_id " + params.Order
	}
}
