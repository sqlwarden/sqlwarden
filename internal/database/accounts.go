package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
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
