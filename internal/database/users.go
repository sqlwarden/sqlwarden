package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID             int       `db:"id"`
	Created        time.Time `db:"created"`
	Email          string    `db:"email"`
	HashedPassword string    `db:"hashed_password"`
}

func (db *DB) InsertUser(email, hashedPassword string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var id int

	query := `
		INSERT INTO users (created, email, hashed_password)
		VALUES ($1, $2, $3)
		RETURNING id`

	err := db.GetContext(ctx, &id, query, time.Now(), email, hashedPassword)
	if err != nil {
		return 0, err
	}

	return id, err
}

func (db *DB) GetUser(id int) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User

	query := `SELECT * FROM users WHERE id = $1`

	err := db.GetContext(ctx, &user, query, id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}

	return user, true, err
}

func (db *DB) GetUserByEmail(email string) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	var user User

	query := `SELECT * FROM users WHERE LOWER(email) = LOWER($1)`

	err := db.GetContext(ctx, &user, query, email)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}

	return user, true, err
}

func (db *DB) UpdateUserHashedPassword(id int, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	query := `UPDATE users SET hashed_password = $1 WHERE id = $2`

	_, err := db.ExecContext(ctx, query, hashedPassword, id)
	return err
}
