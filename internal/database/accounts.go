package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Account struct {
	ID        string    `bun:",pk"                    json:"id"`
	Email     string    `bun:",notnull,unique"        json:"email"`
	Name      string    `bun:",notnull"               json:"name"`
	Password  *string   `bun:",nullzero"              json:"-"`
	IsActive  bool      `bun:",notnull,default:true"  json:"is_active"`
	CreatedAt time.Time `bun:",notnull"               json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"               json:"updated_at"`
}

func (db *DB) InsertAccount(email, name string, password *string) (Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	account := Account{
		ID:        newID(),
		Email:     email,
		Name:      name,
		Password:  password,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err := db.NewInsert().
		Model(&account).
		Exec(ctx)
	if err != nil {
		return Account{}, err
	}

	return account, nil
}

func (db *DB) GetAccount(id string) (Account, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var account Account
	err := db.NewSelect().
		Model(&account).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}

	return account, true, nil
}

func (db *DB) GetAccountByEmail(email string) (Account, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var account Account
	err := db.NewSelect().
		Model(&account).
		Where("LOWER(email) = LOWER(?)", email).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}

	return account, true, nil
}

func (db *DB) UpdateAccountPassword(id, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Account)(nil)).
		Set("password = ?", hashedPassword).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (db *DB) DeactivateAccount(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Account)(nil)).
		Set("is_active = ?", false).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)

	return err
}
