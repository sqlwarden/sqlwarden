package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type RefreshToken struct {
	ID            string     `bun:",pk"             json:"-"`
	AccountID     int64      `bun:",notnull"        json:"-"`
	AuthSessionID string     `bun:",nullzero"       json:"-"`
	TokenHash     string     `bun:",notnull,unique" json:"-"`
	Family        string     `bun:",notnull"        json:"-"`
	ExpiresAt     time.Time  `bun:",notnull"        json:"-"`
	RevokedAt     *time.Time `bun:",nullzero"       json:"-"`
	CreatedAt     time.Time  `bun:",notnull"        json:"-"`
	UserAgent     string     `bun:",nullzero"       json:"-"`
	IPAddress     string     `bun:",nullzero"       json:"-"`
}

func (db *DB) InsertRefreshToken(ctx context.Context, accountID int64, authSessionID, tokenHash, family string, expiresAt time.Time, ua, ip string) (RefreshToken, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.InsertRefreshTokenWithExecutor(ctx, db.DB, accountID, authSessionID, tokenHash, family, expiresAt, ua, ip)
}

// InsertRefreshTokenWithExecutor inserts a refresh token using exec for transaction composition.
func (db *DB) InsertRefreshTokenWithExecutor(ctx context.Context, exec bun.IDB, accountID int64, authSessionID, tokenHash, family string, expiresAt time.Time, ua, ip string) (RefreshToken, error) {
	token := RefreshToken{
		ID:            newID(),
		AccountID:     accountID,
		AuthSessionID: authSessionID,
		TokenHash:     tokenHash,
		Family:        family,
		ExpiresAt:     expiresAt,
		CreatedAt:     time.Now(),
		UserAgent:     ua,
		IPAddress:     ip,
	}

	_, err := exec.NewInsert().
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

	return db.RevokeRefreshTokenWithExecutor(ctx, db.DB, id)
}

// RevokeRefreshTokenWithExecutor revokes a refresh token using exec for transaction composition.
func (db *DB) RevokeRefreshTokenWithExecutor(ctx context.Context, exec bun.IDB, id string) error {
	_, err := exec.NewUpdate().
		Model((*RefreshToken)(nil)).
		Set("revoked_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)

	return err
}

// CreateAuthSessionWithRefreshToken atomically creates an auth session and its initial refresh token.
func (db *DB) CreateAuthSessionWithRefreshToken(ctx context.Context, accountID int64, expiresAt time.Time, userAgent, ipAddress, refreshTokenHash, refreshTokenFamily string) (AuthSession, RefreshToken, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var authSession AuthSession
	var refreshToken RefreshToken
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		authSession, err = db.InsertAuthSessionWithExecutor(ctx, tx, accountID, expiresAt, userAgent, ipAddress)
		if err != nil {
			return err
		}
		refreshToken, err = db.InsertRefreshTokenWithExecutor(ctx, tx, accountID, authSession.ID, refreshTokenHash, refreshTokenFamily, expiresAt, userAgent, ipAddress)
		return err
	})
	if err != nil {
		return AuthSession{}, RefreshToken{}, err
	}

	return authSession, refreshToken, nil
}

// RotateRefreshToken atomically revokes oldTokenID and inserts a replacement token.
func (db *DB) RotateRefreshToken(ctx context.Context, oldTokenID string, accountID int64, authSessionID, tokenHash, family string, expiresAt time.Time, ua, ip string) (RefreshToken, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var refreshToken RefreshToken
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := db.RevokeRefreshTokenWithExecutor(ctx, tx, oldTokenID); err != nil {
			return err
		}
		var err error
		refreshToken, err = db.InsertRefreshTokenWithExecutor(ctx, tx, accountID, authSessionID, tokenHash, family, expiresAt, ua, ip)
		return err
	})
	if err != nil {
		return RefreshToken{}, err
	}

	return refreshToken, nil
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
