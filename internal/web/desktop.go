package web

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/token"
	"github.com/uptrace/bun"
)

const (
	desktopAccountEmail = "local@sqlwarden.invalid"
	desktopAccountName  = "Local User"
	desktopSessionTTL   = 7 * 24 * time.Hour
)

var (
	ErrDesktopModeRequired        = errors.New("desktop mode is required")
	ErrDesktopDatabaseNotPristine = errors.New("desktop bootstrap requires a fresh application database")
	ErrDesktopIdentityInvalid     = errors.New("desktop installation identity is invalid")
)

// DesktopIdentity contains stable local resources required to enter the IDE.
type DesktopIdentity struct {
	AccountID   int64  `json:"account_id"`
	OrgID       int64  `json:"org_id"`
	OrgSlug     string `json:"org_slug"`
	WorkspaceID *int64 `json:"workspace_id,omitempty"`
}

// DesktopSession is returned to the native bridge and never exposed over HTTP.
type DesktopSession struct {
	AccessToken   string          `json:"access_token"`
	AuthSessionID string          `json:"auth_session_id"`
	Identity      DesktopIdentity `json:"identity"`
}

// BootstrapDesktop creates or validates the passwordless local desktop
// identity. All normal authorization records are seeded so desktop mode uses
// the same permission checks as server mode.
func (app *application) BootstrapDesktop(ctx context.Context) (DesktopIdentity, error) {
	if !app.config.isDesktop() {
		return DesktopIdentity{}, ErrDesktopModeRequired
	}

	var installation database.DesktopInstallation
	var workspaceID *int64
	err := app.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var found bool
		var err error
		installation, found, err = database.GetDesktopInstallationWithExecutor(ctx, tx)
		if err != nil {
			return err
		}
		if found {
			accountValid, err := tx.NewSelect().Model((*database.Account)(nil)).
				Where("id = ? AND is_active = ?", installation.AccountID, true).Exists(ctx)
			if err != nil {
				return err
			}
			orgValid, err := tx.NewSelect().Model((*database.Organization)(nil)).
				Where("id = ?", installation.OrgID).Exists(ctx)
			if err != nil {
				return err
			}
			adminValid, err := tx.NewSelect().Model((*database.InstanceAdmin)(nil)).
				Where("account_id = ?", installation.AccountID).Exists(ctx)
			if err != nil {
				return err
			}
			memberValid, err := tx.NewSelect().Model((*database.OrgMember)(nil)).
				Where("org_id = ? AND account_id = ?", installation.OrgID, installation.AccountID).Exists(ctx)
			if err != nil {
				return err
			}
			if !accountValid || !orgValid || !adminValid || !memberValid {
				return ErrDesktopIdentityInvalid
			}
			var workspace database.Workspace
			err = tx.NewSelect().Model(&workspace).
				Where("owner_type = ? AND owner_id = ?", "org", installation.OrgID).
				OrderExpr("id ASC").Limit(1).Scan(ctx)
			if err == nil {
				workspaceID = &workspace.ID
				return nil
			}
			if !errors.Is(err, stdsql.ErrNoRows) {
				return err
			}
			return nil
		}

		pristine, err := database.IsDesktopBootstrapPristineWithExecutor(ctx, tx)
		if err != nil {
			return err
		}
		if !pristine {
			return ErrDesktopDatabaseNotPristine
		}
		account, err := app.db.InsertAccountWithExecutor(ctx, tx, desktopAccountEmail, desktopAccountName, nil)
		if err != nil {
			return err
		}
		if err := app.db.InsertInstanceAdminWithExecutor(ctx, tx, account.ID); err != nil {
			return err
		}
		org, err := app.createOwnedOrganizationWithExecutor(ctx, tx, singleUserDefaultOrgSlug, singleUserDefaultOrgName, account.ID)
		if err != nil {
			return err
		}
		installation, err = database.InsertDesktopInstallationWithExecutor(ctx, tx, account.ID, org.ID)
		return err
	})
	if err != nil {
		return DesktopIdentity{}, err
	}

	account, accountFound, err := app.db.GetAccount(ctx, installation.AccountID)
	if err != nil {
		return DesktopIdentity{}, err
	}
	org, orgFound, err := app.db.GetOrg(ctx, installation.OrgID)
	if err != nil {
		return DesktopIdentity{}, err
	}
	isAdmin, err := app.db.IsInstanceAdmin(ctx, installation.AccountID)
	if err != nil {
		return DesktopIdentity{}, err
	}
	if !accountFound || !account.IsActive || !orgFound || !isAdmin {
		return DesktopIdentity{}, ErrDesktopIdentityInvalid
	}
	app.enforcer.InvalidateOrgPolicy(org.ID)
	app.enforcer.InvalidatePrincipals(org.ID, account.ID)
	if !app.enforcer.Can(ctx, account.ID, org.ID, "org", "org", org.ID, access.PermOrgRead) {
		return DesktopIdentity{}, fmt.Errorf("%w: local account lacks organization access", ErrDesktopIdentityInvalid)
	}
	return DesktopIdentity{AccountID: account.ID, OrgID: org.ID, OrgSlug: org.Slug, WorkspaceID: workspaceID}, nil
}

// NewDesktopSession creates a native-only auth session for the local account.
func (app *application) NewDesktopSession(ctx context.Context) (DesktopSession, error) {
	identity, err := app.BootstrapDesktop(ctx)
	if err != nil {
		return DesktopSession{}, err
	}
	account, found, err := app.db.GetAccount(ctx, identity.AccountID)
	if err != nil {
		return DesktopSession{}, err
	}
	if !found || !account.IsActive {
		return DesktopSession{}, ErrDesktopIdentityInvalid
	}
	session, err := app.db.InsertAuthSession(ctx, account.ID, time.Now().Add(desktopSessionTTL), "SQLWarden Desktop", "desktop")
	if err != nil {
		return DesktopSession{}, err
	}
	accessToken, err := app.issueDesktopAccessToken(ctx, account, session)
	if err != nil {
		_ = app.db.RevokeAuthSession(ctx, session.ID, &account.ID, "desktop_token_issue_failed")
		return DesktopSession{}, err
	}
	return DesktopSession{AccessToken: accessToken, AuthSessionID: session.ID, Identity: identity}, nil
}

// RefreshDesktopSession reuses a still-valid native session or replaces it.
func (app *application) RefreshDesktopSession(ctx context.Context, authSessionID string) (DesktopSession, error) {
	identity, err := app.BootstrapDesktop(ctx)
	if err != nil {
		return DesktopSession{}, err
	}
	account, found, err := app.db.GetAccount(ctx, identity.AccountID)
	if err != nil {
		return DesktopSession{}, err
	}
	if !found || !account.IsActive {
		return DesktopSession{}, ErrDesktopIdentityInvalid
	}
	session, found, err := app.db.GetAuthSession(ctx, authSessionID, account.ID)
	if err != nil {
		return DesktopSession{}, err
	}
	if !found || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		return app.NewDesktopSession(ctx)
	}
	accessToken, err := app.issueDesktopAccessToken(ctx, account, session)
	if err != nil {
		return DesktopSession{}, err
	}
	return DesktopSession{AccessToken: accessToken, AuthSessionID: session.ID, Identity: identity}, nil
}

func (app *application) issueDesktopAccessToken(ctx context.Context, account database.Account, session database.AuthSession) (string, error) {
	settings, err := app.runtimeSettingsService().effectiveForOrg(ctx, nil)
	if err != nil {
		return "", err
	}
	accessToken, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), session.ID, account.Email, account.Name, app.config.JWT.SecretKey, settings.JWTAccessTokenTTL)
	return accessToken, err
}

// RevokeDesktopSession invalidates the native session during app shutdown.
func (app *application) RevokeDesktopSession(ctx context.Context, authSessionID string) error {
	installation, found, err := app.db.GetDesktopInstallation(ctx)
	if err != nil || !found || authSessionID == "" {
		return err
	}
	return app.db.RevokeAuthSession(ctx, authSessionID, &installation.AccountID, "desktop_shutdown")
}

// BackupDesktopDatabase asks SQLite to produce a transactionally consistent
// snapshot without stopping the in-process application.
func (app *application) BackupDesktopDatabase(ctx context.Context, destination string) error {
	if !app.config.isDesktop() {
		return ErrDesktopModeRequired
	}
	if destination == "" {
		return errors.New("desktop backup destination is required")
	}
	_, err := app.db.ExecContext(ctx, "VACUUM INTO ?", destination)
	if err != nil {
		return fmt.Errorf("create desktop database snapshot: %w", err)
	}
	return nil
}
