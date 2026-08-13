package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

const (
	InvitationDeliverySent     = "sent"
	InvitationDeliveryFailed   = "failed"
	InvitationDeliveryDisabled = "disabled"
)

type OrganizationInvitation struct {
	bun.BaseModel `bun:"table:organization_invitations"`

	ID                    string     `bun:",pk" json:"id"`
	OrgID                 int64      `bun:",notnull" json:"org_id"`
	Email                 string     `bun:",notnull" json:"email"`
	NormalizedEmail       string     `bun:",notnull" json:"-"`
	InvitedByAccountID    *int64     `bun:",nullzero" json:"invited_by_account_id,omitempty"`
	TokenHash             string     `bun:",notnull,unique" json:"-"`
	ExpiresAt             time.Time  `bun:",notnull" json:"expires_at"`
	AcceptedAt            *time.Time `bun:",nullzero" json:"accepted_at,omitempty"`
	AcceptedByAccountID   *int64     `bun:",nullzero" json:"accepted_by_account_id,omitempty"`
	RevokedAt             *time.Time `bun:",nullzero" json:"revoked_at,omitempty"`
	LastDeliveryStatus    string     `bun:",notnull" json:"delivery_status"`
	LastDeliveryAttemptAt *time.Time `bun:",nullzero" json:"last_delivery_attempt_at,omitempty"`
	CreatedAt             time.Time  `bun:",notnull" json:"created_at"`
	UpdatedAt             time.Time  `bun:",notnull" json:"updated_at"`
}

type OrganizationInvitationListItem struct {
	OrganizationInvitation
	InviterName string `bun:"inviter_name" json:"inviter_name,omitempty"`
	Status      string `bun:"-" json:"status"`
}

type ListOrganizationInvitationsParams struct {
	OrgID    int64
	Search   string
	Page     int
	PageSize int
}

func NormalizeInvitationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (db *DB) InsertOrganizationInvitation(ctx context.Context, orgID int64, email, tokenHash string, invitedBy int64, expiresAt time.Time) (OrganizationInvitation, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	invitation := OrganizationInvitation{
		ID:                 newID(),
		OrgID:              orgID,
		Email:              strings.TrimSpace(email),
		NormalizedEmail:    NormalizeInvitationEmail(email),
		InvitedByAccountID: &invitedBy,
		TokenHash:          tokenHash,
		ExpiresAt:          expiresAt,
		LastDeliveryStatus: InvitationDeliveryDisabled,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err := db.NewInsert().Model(&invitation).Exec(ctx)
	return invitation, err
}

func (db *DB) GetOrganizationInvitation(ctx context.Context, orgID int64, id string) (OrganizationInvitation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var invitation OrganizationInvitation
	err := db.NewSelect().Model(&invitation).Where("org_id = ? AND id = ?", orgID, id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrganizationInvitation{}, false, nil
	}
	return invitation, err == nil, err
}

func (db *DB) GetOrganizationInvitationByTokenHash(ctx context.Context, tokenHash string) (OrganizationInvitation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var invitation OrganizationInvitation
	err := db.NewSelect().Model(&invitation).Where("token_hash = ?", tokenHash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrganizationInvitation{}, false, nil
	}
	return invitation, err == nil, err
}

func (db *DB) ListOrganizationInvitationsPage(ctx context.Context, params ListOrganizationInvitationsParams) (response.Paginated[OrganizationInvitationListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	params.Search = strings.TrimSpace(params.Search)
	var invitations []OrganizationInvitationListItem
	query := db.NewSelect().
		TableExpr("organization_invitations AS invitation").
		ColumnExpr("invitation.*").
		ColumnExpr("COALESCE(inviter.name, '') AS inviter_name").
		Join("LEFT JOIN accounts AS inviter ON inviter.id = invitation.invited_by_account_id").
		Where("invitation.org_id = ?", params.OrgID).
		Where("invitation.accepted_at IS NULL").
		Where("invitation.revoked_at IS NULL")
	if params.Search != "" {
		query = query.Where("LOWER(invitation.email) LIKE ?", "%"+strings.ToLower(params.Search)+"%")
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return response.Paginated[OrganizationInvitationListItem]{}, err
	}
	err = query.OrderExpr("invitation.created_at DESC").
		Limit(params.PageSize).
		Offset((params.Page-1)*params.PageSize).
		Scan(ctx, &invitations)
	if err != nil {
		return response.Paginated[OrganizationInvitationListItem]{}, err
	}
	now := time.Now()
	for i := range invitations {
		invitations[i].Status = "pending"
		if !now.Before(invitations[i].ExpiresAt) {
			invitations[i].Status = "expired"
		}
	}
	if invitations == nil {
		invitations = []OrganizationInvitationListItem{}
	}
	return response.Paginated[OrganizationInvitationListItem]{Items: invitations, Page: params.Page, PageSize: params.PageSize, Total: total}, nil
}

func (db *DB) RotateOrganizationInvitation(ctx context.Context, orgID int64, id, tokenHash string, expiresAt time.Time) (OrganizationInvitation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	var invitation OrganizationInvitation
	err := db.NewUpdate().Model(&invitation).
		Set("token_hash = ?", tokenHash).
		Set("expires_at = ?", expiresAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ? AND org_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", id, orgID).
		Returning("*").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrganizationInvitation{}, false, nil
	}
	return invitation, err == nil, err
}

func (db *DB) UpdateOrganizationInvitationDelivery(ctx context.Context, id, status string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	now := time.Now()
	_, err := db.NewUpdate().Model((*OrganizationInvitation)(nil)).
		Set("last_delivery_status = ?", status).
		Set("last_delivery_attempt_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).Exec(ctx)
	return err
}

func (db *DB) RevokeOrganizationInvitation(ctx context.Context, orgID int64, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	now := time.Now()
	result, err := db.NewUpdate().Model((*OrganizationInvitation)(nil)).
		Set("revoked_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ? AND org_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", id, orgID).Exec(ctx)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (db *DB) MarkOrganizationInvitationAcceptedWithExecutor(ctx context.Context, exec bun.IDB, id string, accountID int64) (bool, error) {
	now := time.Now()
	result, err := exec.NewUpdate().Model((*OrganizationInvitation)(nil)).
		Set("accepted_at = ?", now).
		Set("accepted_by_account_id = ?", accountID).
		Set("updated_at = ?", now).
		Where("id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", id, now).Exec(ctx)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
