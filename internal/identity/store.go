package identity

import (
	"context"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// Store is the persistence port of the identity service.
type Store interface {
	HasAnyInstanceAdmin(ctx context.Context) (bool, error)
	ListInstanceAdminsPage(ctx context.Context, params database.ListInstanceAdminsParams) (response.Paginated[database.InstanceAdmin], error)
	InsertInstanceAdmin(ctx context.Context, accountID int64) error
	// RemoveInstanceAdmin returns database.ErrLastInstanceAdmin instead of
	// removing the only remaining admin.
	RemoveInstanceAdmin(ctx context.Context, accountID int64) error

	ListOrganizationsPage(ctx context.Context, params database.ListOrganizationsParams) (response.Paginated[database.OrganizationListItem], error)
	GetOrgBySlug(ctx context.Context, slug string) (database.Organization, bool, error)
	ListAccountsPage(ctx context.Context, params database.ListAccountsParams) (response.Paginated[database.Account], error)

	GetAccount(ctx context.Context, id int64) (database.Account, bool, error)
	GetAccountByEmail(ctx context.Context, email string) (database.Account, bool, error)
	InsertAccount(ctx context.Context, email, name string, password *string) (database.Account, error)
	UpdateAccountName(ctx context.Context, id int64, name string) (database.Account, error)
	ChangeAccountPassword(ctx context.Context, accountID int64, hashedPassword, keepSessionID, reason string) error
	GetAccountOrgs(ctx context.Context, accountID int64) ([]database.Organization, error)
	ListAccountOrgsPage(ctx context.Context, params database.ListAccountOrgsParams) (response.Paginated[database.AccountOrganizationListItem], error)

	CreateAuthSessionWithRefreshToken(ctx context.Context, accountID int64, expiresAt time.Time, userAgent, ipAddress, tokenHash, family string, auth database.SessionAuth) (database.AuthSession, database.RefreshToken, error)
	GetAuthSession(ctx context.Context, id string, accountID int64) (database.AuthSession, bool, error)
	RevokeAuthSession(ctx context.Context, id string, revokedBy *int64, reason string) error
	RevokeAuthSessionsForAccount(ctx context.Context, accountID int64, revokedBy *int64, reason string) error
	ListAuthSessionsPage(ctx context.Context, params database.ListAuthSessionsParams) (response.Paginated[database.AuthSession], error)

	GetRefreshTokenByHash(ctx context.Context, hash string) (database.RefreshToken, bool, error)
	RevokeRefreshToken(ctx context.Context, id string) error
	RevokeFamilyTokens(ctx context.Context, family string) error
	RotateRefreshToken(ctx context.Context, oldID string, accountID int64, authSessionID, tokenHash, family string, expiresAt time.Time, userAgent, ipAddress string) (database.RefreshToken, error)

	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	EnsureOrgAccessSession(ctx context.Context, authSessionID string, orgID, accountID int64, expiresAt time.Time) error
	GetOrgAccessSession(ctx context.Context, authSessionID string, orgID, accountID int64) (database.OrgAccessSession, bool, error)
	ListOrgAccessSessionsPage(ctx context.Context, orgID, accountID int64, page, pageSize int) (response.Paginated[database.OrgAccessSession], error)
	GetOrgAccessSessionByID(ctx context.Context, id string, orgID, accountID int64) (database.OrgAccessSession, bool, error)
	RevokeOrgAccessSession(ctx context.Context, id string, orgID int64, revokedBy *int64, reason string) error
}

// TokenPolicy supplies the access token lifetime in effect for new tokens.
type TokenPolicy interface {
	AccessTokenTTL(ctx context.Context) (time.Duration, error)
}
