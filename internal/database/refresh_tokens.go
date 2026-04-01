package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type RefreshToken struct {
	ID        string     `bun:",pk"             json:"-"`
	AccountID int64      `bun:",notnull"        json:"-"`
	TokenHash string     `bun:",notnull,unique" json:"-"`
	Family    string     `bun:",notnull"        json:"-"`
	ExpiresAt time.Time  `bun:",notnull"        json:"-"`
	RevokedAt *time.Time `bun:",nullzero"       json:"-"`
	CreatedAt time.Time  `bun:",notnull"        json:"-"`
	UserAgent string     `bun:",nullzero"       json:"-"`
	IPAddress string     `bun:",nullzero"       json:"-"`
}

func (db *DB) InsertRefreshToken(ctx context.Context, accountID int64, tokenHash, family string, expiresAt time.Time, ua, ip string) (RefreshToken, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	token := RefreshToken{
		ID:        newID(),
		AccountID: accountID,
		TokenHash: tokenHash,
		Family:    family,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		UserAgent: ua,
		IPAddress: ip,
	}

	_, err := db.NewInsert().
		Model(&token).
		Exec(ctx)
	if err != nil {
		return RefreshToken{}, err
	}

	return token, nil
}

func (db *DB) GetRefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var token RefreshToken
	err := db.NewSelect().
		Model(&token).
		Where("token_hash = ?", hash).
		Scan(ctx)

	if errors.Is(err, sql.ErrNoRows) {
		return RefreshToken{}, false, nil
	}
	if err != nil {
		return RefreshToken{}, false, err
	}

	return token, true, nil
}

func (db *DB) RevokeRefreshToken(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*RefreshToken)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

func (db *DB) RevokeFamilyTokens(ctx context.Context, family string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*RefreshToken)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("family = ? AND revoked_at IS NULL", family).
		Exec(ctx)

	return err
}

func (db *DB) DeleteExpiredRefreshTokens(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().
		Model((*RefreshToken)(nil)).
		Where("expires_at < ?", time.Now()).
		Exec(ctx)

	return err
}
