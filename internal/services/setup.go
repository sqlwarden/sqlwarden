package services

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/uptrace/bun"
)

// firstRunStore writes first-run setup in one transaction, so a failure at any
// step leaves no account, admin, organization, or session behind.
type firstRunStore struct {
	db       *database.DB
	enforcer *access.Enforcer
}

func (s firstRunStore) CreateFirstRun(ctx context.Context, in identity.FirstRun) (identity.FirstRunRecords, error) {
	if s.enforcer == nil {
		return identity.FirstRunRecords{}, errors.New("services: enforcer is required to seed organization roles")
	}
	var out identity.FirstRunRecords
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// The service checked before hashing the password. Re-checking under
		// the setup lock makes concurrent setups serialize, so exactly one of
		// them finds no instance admin.
		if err := s.db.LockFirstRunSetup(ctx, tx); err != nil {
			return err
		}
		configured, err := tx.NewSelect().Model((*database.InstanceAdmin)(nil)).Exists(ctx)
		if err != nil {
			return err
		}
		if configured {
			return identity.ErrAlreadyConfigured
		}
		out.Account, err = s.db.InsertAccountWithExecutor(ctx, tx, in.Email, in.Name, in.PasswordHash)
		if err != nil {
			return err
		}
		if err := s.db.InsertInstanceAdminWithExecutor(ctx, tx, out.Account.ID); err != nil {
			return err
		}
		out.Org, err = createOwnedOrgWithExecutor(ctx, s.db, s.enforcer, tx, in.OrgSlug, in.OrgName, out.Account.ID)
		if err != nil {
			return err
		}
		out.AuthSession, err = s.db.InsertAuthSessionWithExecutor(ctx, tx, out.Account.ID, in.SessionExpiresAt, in.UserAgent, in.IPAddress, in.Auth)
		return err
	})
	if err != nil {
		return identity.FirstRunRecords{}, err
	}
	return out, nil
}
