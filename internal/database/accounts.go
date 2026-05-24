package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
)

type Account struct {
	ID        int64     `bun:",pk,autoincrement"      json:"id"`
	Email     string    `bun:",notnull,unique"        json:"email"`
	Name      string    `bun:",notnull"               json:"name"`
	Password  *string   `bun:",nullzero"              json:"-"`
	IsActive  bool      `bun:",notnull,default:true"  json:"is_active"`
	CreatedAt time.Time `bun:",notnull"               json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"               json:"updated_at"`
}

type ListAccountsParams struct {
	ExcludeOrgID int64
	Search       string
	Sort         string
	Order        string
	Page         int
	PageSize     int
}

func (db *DB) InsertAccount(ctx context.Context, email, name string, password *string) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	account := Account{
		Email:     email,
		Name:      name,
		Password:  password,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := db.NewInsert().Model(&account).Returning("id").Exec(ctx)
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (db *DB) GetAccount(ctx context.Context, id int64) (Account, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var account Account
	err := db.NewSelect().Model(&account).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	return account, true, nil
}

func (db *DB) GetAccountByEmail(ctx context.Context, email string) (Account, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var account Account
	err := db.NewSelect().Model(&account).Where("LOWER(email) = LOWER(?)", email).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	return account, true, nil
}

func (db *DB) ListAccountsPage(ctx context.Context, params ListAccountsParams) (response.Paginated[Account], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeAccountListParams(params)

	var accounts []Account
	query := db.NewSelect().Model(&accounts)
	if params.ExcludeOrgID > 0 {
		query = query.Where(`
NOT EXISTS (
	SELECT 1
	FROM org_members AS om
	WHERE om.org_id = ? AND om.account_id = account.id
)`, params.ExcludeOrgID)
	}
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query = query.Where("(LOWER(email) LIKE ? OR LOWER(name) LIKE ?)", search, search)
	}
	err := query.OrderExpr(fmt.Sprintf("%s %s, id %s", accountSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))).Scan(ctx)
	if err != nil {
		return response.Paginated[Account]{}, err
	}
	return response.PaginateItems(accounts, params.Page, params.PageSize), nil
}

func (db *DB) UpdateAccountPassword(ctx context.Context, id int64, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Account)(nil)).
		Set("password = ?", hashedPassword).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) UpdateAccountName(ctx context.Context, id int64, name string) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var account Account
	err := db.NewUpdate().
		Model(&account).
		Set("name = ?", name).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Returning("*").
		Scan(ctx)
	if err != nil {
		return Account{}, err
	}
	return account, nil
}

func (db *DB) DeactivateAccount(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Account)(nil)).
		Set("is_active = ?", false).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func normalizeAccountListParams(params ListAccountsParams) ListAccountsParams {
	switch params.Sort {
	case "id", "email", "name", "created_at":
	default:
		params.Sort = "created_at"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "desc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func accountSortColumn(sort string) string {
	switch sort {
	case "id":
		return "id"
	case "email":
		return "email"
	case "name":
		return "name"
	default:
		return "created_at"
	}
}
