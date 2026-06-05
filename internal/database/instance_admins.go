package database

import (
	"context"
	"errors"
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

var ErrLastInstanceAdmin = errors.New("cannot remove the last instance admin")

type ListInstanceAdminsParams struct {
	Search   string
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

	return db.InsertInstanceAdminWithExecutor(ctx, db.DB, accountID)
}

// InsertInstanceAdminWithExecutor inserts an instance admin using exec for transaction composition.
func (db *DB) InsertInstanceAdminWithExecutor(ctx context.Context, exec bun.IDB, accountID int64) error {
	admin := &InstanceAdmin{AccountID: accountID, CreatedAt: time.Now()}
	_, err := exec.NewInsert().Model(admin).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

func (db *DB) RemoveInstanceAdmin(ctx context.Context, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		count, err := tx.NewSelect().Model((*InstanceAdmin)(nil)).Count(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastInstanceAdmin
		}

		_, err = tx.NewDelete().Model((*InstanceAdmin)(nil)).
			Where("account_id = ?", accountID).
			Exec(ctx)
		return err
	})
}

func (db *DB) ListInstanceAdminsPage(ctx context.Context, params ListInstanceAdminsParams) (response.Paginated[InstanceAdmin], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeInstanceAdminListParams(params)

	var admins []InstanceAdmin
	query := db.NewSelect().Model(&admins).
		Relation("Account").
		OrderExpr(instanceAdminOrderExpr(params))
	if params.Search != "" {
		search := "%" + params.Search + "%"
		query = query.Where("(LOWER(account.email) LIKE LOWER(?) OR LOWER(account.name) LIKE LOWER(?))", search, search)
	}
	err := query.Scan(ctx)
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
