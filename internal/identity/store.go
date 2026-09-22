package identity

import (
	"context"

	"github.com/sqlwarden/internal/database"
)

// DatabaseStore adapts the metadata database to [Store]. It exists because the
// database package names its account reads after the table rather than after
// the identity use cases.
type DatabaseStore struct {
	db *database.DB
}

// NewDatabaseStore returns a [Store] backed by the metadata database.
func NewDatabaseStore(db *database.DB) *DatabaseStore { return &DatabaseStore{db: db} }

// HasAnyInstanceAdmin implements [Store].
func (s *DatabaseStore) HasAnyInstanceAdmin(ctx context.Context) (bool, error) {
	return s.db.HasAnyInstanceAdmin(ctx)
}

// AccountByEmail implements [Store].
func (s *DatabaseStore) AccountByEmail(ctx context.Context, email string) (database.Account, bool, error) {
	return s.db.GetAccountByEmail(ctx, email)
}

// Account implements [Store].
func (s *DatabaseStore) Account(ctx context.Context, accountID int64) (database.Account, bool, error) {
	return s.db.GetAccount(ctx, accountID)
}

// InsertAccount implements [Store].
func (s *DatabaseStore) InsertAccount(ctx context.Context, email, name string, hashedPassword *string) (database.Account, error) {
	return s.db.InsertAccount(ctx, email, name, hashedPassword)
}

// UpdateAccountName implements [Store].
func (s *DatabaseStore) UpdateAccountName(ctx context.Context, accountID int64, name string) (database.Account, error) {
	return s.db.UpdateAccountName(ctx, accountID, name)
}

// UpdateAccountPassword implements [Store].
func (s *DatabaseStore) UpdateAccountPassword(ctx context.Context, accountID int64, hashedPassword string) error {
	return s.db.UpdateAccountPassword(ctx, accountID, hashedPassword)
}

var _ Store = (*DatabaseStore)(nil)
