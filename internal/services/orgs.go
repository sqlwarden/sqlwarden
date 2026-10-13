package services

import (
	"context"
	"errors"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/settings"
	"github.com/sqlwarden/internal/smtp"
	"github.com/uptrace/bun"
)

// orgStore adapts the metadata database and the enforcer's seeding to the
// orgs service store.
type orgStore struct {
	*database.DB
	enforcer *access.Enforcer
}

func (s orgStore) CreateOwnedOrg(ctx context.Context, slug, name string, ownerAccountID int64) (database.Organization, error) {
	if s.enforcer == nil {
		return database.Organization{}, errors.New("services: enforcer is required to seed organization roles")
	}
	var org database.Organization
	err := s.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		org, err = createOwnedOrgWithExecutor(ctx, s.DB, s.enforcer, tx, slug, name, ownerAccountID)
		return err
	})
	if err != nil {
		return database.Organization{}, err
	}
	return org, nil
}

func (s orgStore) ReplaceOrgMemberBuiltinRole(ctx context.Context, orgID, accountID, roleID int64, replacedRoleIDs []int64, grantorID int64) error {
	return s.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if len(replacedRoleIDs) > 0 {
			if _, err := tx.NewDelete().
				Model((*database.RoleBinding)(nil)).
				Where("org_id = ? AND subject_type = ? AND subject_id = ? AND resource_type = ? AND resource_id = ? AND role_id IN (?)",
					orgID, "account", accountID, "org", orgID, bun.List(replacedRoleIDs)).
				Exec(ctx); err != nil {
				return err
			}
		}
		binding := database.RoleBinding{
			OrgID:        orgID,
			RoleID:       roleID,
			SubjectType:  "account",
			SubjectID:    accountID,
			ResourceType: "org",
			ResourceID:   orgID,
			CreatedBy:    &grantorID,
			CreatedAt:    time.Now(),
		}
		_, err := tx.NewInsert().Model(&binding).Ignore().Exec(ctx)
		return err
	})
}

func (s orgStore) AcceptOrganizationInvitation(ctx context.Context, invitation database.OrganizationInvitation, accountID int64, newAccount *orgs.InvitedAccount) (database.Account, error) {
	var account database.Account
	err := s.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		account = database.Account{ID: accountID}
		if newAccount != nil {
			hash := newAccount.PasswordHash
			created, err := s.InsertAccountWithExecutor(ctx, tx, newAccount.Email, newAccount.Name, &hash)
			if err != nil {
				return err
			}
			account = created
		}
		if err := s.AddOrgMemberWithExecutor(ctx, tx, invitation.OrgID, account.ID); err != nil {
			return err
		}
		accepted, err := s.MarkOrganizationInvitationAcceptedWithExecutor(ctx, tx, invitation.ID, account.ID)
		if err != nil {
			return err
		}
		if !accepted {
			return orgs.ErrInvitationUnavailable
		}
		return nil
	})
	if err != nil {
		return database.Account{}, err
	}
	return account, nil
}

// invitationMailer reports disabled delivery as orgs.ErrMailDisabled.
type invitationMailer struct{ live *settings.LiveMailer }

func (m invitationMailer) SendOnce(recipient string, data any, templates ...string) error {
	err := m.live.SendOnce(recipient, data, templates...)
	if errors.Is(err, smtp.ErrDisabled) {
		return orgs.ErrMailDisabled
	}
	return err
}

// instanceBaseURL reads the public base URL from the current instance
// settings, so links follow runtime changes.
type instanceBaseURL struct{ settings *settings.Service }

func (b instanceBaseURL) BaseURL(ctx context.Context) (string, error) {
	in, err := b.settings.Instance(ctx)
	if err != nil {
		return "", err
	}
	return in.BaseURL, nil
}

// createOwnedOrgWithExecutor inserts an organization, adds the owner as a
// member, and seeds its builtin roles and owner binding through exec, so
// callers such as first-run setup can compose it into a larger transaction.
// The caller invalidates the organization's cached policy after commit.
func createOwnedOrgWithExecutor(ctx context.Context, db *database.DB, enforcer *access.Enforcer, exec bun.IDB, slug, name string, ownerAccountID int64) (database.Organization, error) {
	org, err := db.InsertOrgWithExecutor(ctx, exec, slug, name)
	if err != nil {
		return database.Organization{}, err
	}
	if err := db.AddOrgMemberWithExecutor(ctx, exec, org.ID, ownerAccountID); err != nil {
		return database.Organization{}, err
	}
	if err := enforcer.SeedOrgWithExecutor(ctx, exec, org.ID, ownerAccountID); err != nil {
		return database.Organization{}, err
	}
	return org, nil
}
