package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

const sessionLastSeenUpdateInterval = 5 * time.Minute

type AuthSession struct {
	ID                 string     `bun:",pk"       json:"id"`
	AccountID          int64      `bun:",notnull"  json:"account_id"`
	UserAgent          string     `bun:",nullzero" json:"user_agent,omitempty"`
	IPAddress          string     `bun:",nullzero" json:"ip_address,omitempty"`
	CreatedAt          time.Time  `bun:",notnull"  json:"created_at"`
	LastSeenAt         time.Time  `bun:",notnull"  json:"last_seen_at"`
	ExpiresAt          time.Time  `bun:",notnull"  json:"expires_at"`
	RevokedAt          *time.Time `bun:",nullzero" json:"revoked_at,omitempty"`
	RevokedByAccountID *int64     `bun:",nullzero" json:"revoked_by_account_id,omitempty"`
	RevocationReason   string     `bun:",nullzero" json:"revocation_reason,omitempty"`
}

type OrgAccessSession struct {
	ID                 string     `bun:",pk"       json:"id"`
	AuthSessionID      string     `bun:",notnull"  json:"auth_session_id"`
	OrgID              int64      `bun:",notnull"  json:"org_id"`
	AccountID          int64      `bun:",notnull"  json:"account_id"`
	CreatedAt          time.Time  `bun:",notnull"  json:"created_at"`
	LastSeenAt         time.Time  `bun:",notnull"  json:"last_seen_at"`
	ExpiresAt          time.Time  `bun:",notnull"  json:"expires_at"`
	RevokedAt          *time.Time `bun:",nullzero" json:"revoked_at,omitempty"`
	RevokedByAccountID *int64     `bun:",nullzero" json:"revoked_by_account_id,omitempty"`
	RevocationReason   string     `bun:",nullzero" json:"revocation_reason,omitempty"`
}

type ListAuthSessionsParams struct {
	AccountID int64
	Page      int
	PageSize  int
}

func (db *DB) InsertAuthSession(ctx context.Context, accountID int64, expiresAt time.Time, userAgent, ipAddress string) (AuthSession, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	session := AuthSession{
		ID:         newID(),
		AccountID:  accountID,
		UserAgent:  userAgent,
		IPAddress:  ipAddress,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  expiresAt,
	}
	_, err := db.NewInsert().Model(&session).Exec(ctx)
	if err != nil {
		return AuthSession{}, err
	}
	return session, nil
}

func (db *DB) GetAuthSession(ctx context.Context, id string, accountID int64) (AuthSession, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var session AuthSession
	err := db.NewSelect().
		Model(&session).
		Where("id = ? AND account_id = ?", id, accountID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, false, nil
	}
	if err != nil {
		return AuthSession{}, false, err
	}
	return session, true, nil
}

func (db *DB) TouchAuthSession(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*AuthSession)(nil)).
		Set("last_seen_at = ?", time.Now()).
		Where("id = ?", id).
		Where("last_seen_at < ?", time.Now().Add(-sessionLastSeenUpdateInterval)).
		Exec(ctx)
	return err
}

func (db *DB) RevokeAuthSession(ctx context.Context, id string, revokedBy *int64, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*AuthSession)(nil)).
			Set("revoked_at = ?", now).
			Set("revoked_by_account_id = ?", revokedBy).
			Set("revocation_reason = ?", reason).
			Where("id = ? AND revoked_at IS NULL", id).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewUpdate().
			Model((*RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Where("auth_session_id = ? AND revoked_at IS NULL", id).
			Exec(ctx)
		return err
	})
}

func (db *DB) RevokeAuthSessionsForAccount(ctx context.Context, accountID int64, revokedBy *int64, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now()
		_, err := tx.NewUpdate().
			Model((*AuthSession)(nil)).
			Set("revoked_at = ?", now).
			Set("revoked_by_account_id = ?", revokedBy).
			Set("revocation_reason = ?", reason).
			Where("account_id = ? AND revoked_at IS NULL", accountID).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewUpdate().
			Model((*RefreshToken)(nil)).
			Set("revoked_at = ?", now).
			Where("account_id = ? AND revoked_at IS NULL", accountID).
			Exec(ctx)
		return err
	})
}

func (db *DB) ListAuthSessionsPage(ctx context.Context, params ListAuthSessionsParams) (response.Paginated[AuthSession], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}

	var sessions []AuthSession
	err := db.NewSelect().
		Model(&sessions).
		Where("account_id = ?", params.AccountID).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return response.Paginated[AuthSession]{}, err
	}
	return response.PaginateItems(sessions, params.Page, params.PageSize), nil
}

func (db *DB) EnsureOrgAccessSession(ctx context.Context, authSessionID string, orgID, accountID int64, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	existing, found, err := db.getOrgAccessSessionWithExecutor(ctx, db.DB, authSessionID, orgID, accountID)
	if err != nil {
		return err
	}
	if found {
		if existing.RevokedAt != nil || time.Now().After(existing.ExpiresAt) || existing.LastSeenAt.After(time.Now().Add(-sessionLastSeenUpdateInterval)) {
			return nil
		}
		_, err = db.NewUpdate().
			Model((*OrgAccessSession)(nil)).
			Set("last_seen_at = ?", time.Now()).
			Where("id = ?", existing.ID).
			Where("last_seen_at < ?", time.Now().Add(-sessionLastSeenUpdateInterval)).
			Exec(ctx)
		return err
	}

	now := time.Now()
	session := OrgAccessSession{
		ID:            newID(),
		AuthSessionID: authSessionID,
		OrgID:         orgID,
		AccountID:     accountID,
		CreatedAt:     now,
		LastSeenAt:    now,
		ExpiresAt:     expiresAt,
	}
	_, err = db.NewInsert().
		Model(&session).
		On("CONFLICT (auth_session_id, org_id) DO NOTHING").
		Exec(ctx)
	return err
}

func (db *DB) GetOrgAccessSession(ctx context.Context, authSessionID string, orgID, accountID int64) (OrgAccessSession, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.getOrgAccessSessionWithExecutor(ctx, db.DB, authSessionID, orgID, accountID)
}

func (db *DB) getOrgAccessSessionWithExecutor(ctx context.Context, executor bun.IDB, authSessionID string, orgID, accountID int64) (OrgAccessSession, bool, error) {
	var session OrgAccessSession
	err := executor.NewSelect().
		Model(&session).
		Where("auth_session_id = ? AND org_id = ? AND account_id = ?", authSessionID, orgID, accountID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgAccessSession{}, false, nil
	}
	if err != nil {
		return OrgAccessSession{}, false, err
	}
	return session, true, nil
}

func (db *DB) GetOrgAccessSessionByID(ctx context.Context, id string, orgID, accountID int64) (OrgAccessSession, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var session OrgAccessSession
	err := db.NewSelect().
		Model(&session).
		Where("id = ? AND org_id = ? AND account_id = ?", id, orgID, accountID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgAccessSession{}, false, nil
	}
	if err != nil {
		return OrgAccessSession{}, false, err
	}
	return session, true, nil
}

func (db *DB) RevokeOrgAccessSession(ctx context.Context, id string, orgID int64, revokedBy *int64, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*OrgAccessSession)(nil)).
		Set("revoked_at = ?", time.Now()).
		Set("revoked_by_account_id = ?", revokedBy).
		Set("revocation_reason = ?", reason).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", id, orgID).
		Exec(ctx)
	return err
}

func (db *DB) RevokeOrgAccessSessionsForMember(ctx context.Context, orgID, accountID int64, revokedBy *int64, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*OrgAccessSession)(nil)).
		Set("revoked_at = ?", time.Now()).
		Set("revoked_by_account_id = ?", revokedBy).
		Set("revocation_reason = ?", reason).
		Where("org_id = ? AND account_id = ? AND revoked_at IS NULL", orgID, accountID).
		Exec(ctx)
	return err
}

func (db *DB) ListOrgAccessSessionsPage(ctx context.Context, orgID, accountID int64, page, pageSize int) (response.Paginated[OrgAccessSession], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	var sessions []OrgAccessSession
	err := db.NewSelect().
		Model(&sessions).
		Where("org_id = ? AND account_id = ?", orgID, accountID).
		OrderExpr("created_at DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return response.Paginated[OrgAccessSession]{}, err
	}
	return response.PaginateItems(sessions, page, pageSize), nil
}
