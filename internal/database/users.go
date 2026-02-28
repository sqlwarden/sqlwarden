package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID             int64     `bun:",pk,autoincrement" json:"id"`
	Created        time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created"`
	Email          string    `bun:",notnull,unique" json:"email"`
	HashedPassword string    `bun:",notnull" json:"hashed_password"`
}

func (db *DB) InsertUser(email, hashedPassword string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	user := &User{
		Created:        time.Now(),
		Email:          email,
		HashedPassword: hashedPassword,
	}

	_, err := db.NewInsert().
		Model(user).
		Exec(ctx)
	if err != nil {
		return 0, err
	}

	return int(user.ID), nil
}

func (db *DB) GetUser(id int64) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User
	err := db.NewSelect().
		Model(&user).
		Where("id = ?", id).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}

	if err != nil {
		return User{}, false, err
	}

	return user, true, nil
}

func (db *DB) GetUserByEmail(email string) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User
	err := db.NewSelect().
		Model(&user).
		Where("LOWER(email) = LOWER(?)", email).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}

	return user, true, nil
}

func (db *DB) GetUsers() ([]User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	users := []User{}
	err := db.NewSelect().
		Model(&users).
		Order("created DESC").
		Scan(ctx)

	return users, err
}

func (db *DB) UpdateUserHashedPassword(id int64, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*User)(nil)).
		Set("hashed_password = ?", hashedPassword).
		Where("id = ?", id).
		Exec(ctx)

	return err
}
