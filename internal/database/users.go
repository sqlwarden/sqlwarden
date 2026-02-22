package database

import (
	"context"
	"errors"
	"time"

	"github.com/sqlwarden/internal/query"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type User struct {
	ID             int       `json:"id"`
	Created        time.Time `json:"created"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"hashed_password"`
}

func fromQueryUser(u query.User) User {
	return User{
		ID:             int(u.ID),
		Created:        u.Created.Time,
		Email:          u.Email,
		HashedPassword: u.HashedPassword,
	}
}

func (db *DB) InsertUser(email, hashedPassword string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	now := pgtype.Timestamptz{
		Time:  time.Now(),
		Valid: true,
	}

	user, err := db.Queries.InsertUser(ctx, query.InsertUserParams{
		Created:        now,
		Email:          email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		return 0, err
	}

	return int(user.ID), nil
}

func (db *DB) GetUser(id int) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	user, err := db.Queries.GetUser(ctx, int32(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}

	return fromQueryUser(user), true, nil
}

func (db *DB) GetUserByEmail(email string) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	user, err := db.Queries.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}

	return fromQueryUser(user), true, nil
}

func (db *DB) UpdateUserHashedPassword(id int, hashedPassword string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	return db.Queries.UpdateUserHashedPassword(ctx, query.UpdateUserHashedPasswordParams{
		HashedPassword: hashedPassword,
		ID:             int32(id),
	})
}
