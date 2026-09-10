package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

// DesktopInstallation anchors the local desktop identity to stable database IDs.
// The row is absent for server deployments and is a singleton for desktop deployments.
type DesktopInstallation struct {
	bun.BaseModel `bun:"table:desktop_installation"`

	ID        int16     `bun:",pk"`
	AccountID int64     `bun:",notnull,unique"`
	OrgID     int64     `bun:",notnull,unique"`
	CreatedAt time.Time `bun:",notnull"`
	UpdatedAt time.Time `bun:",notnull"`
}

func (db *DB) GetDesktopInstallation(ctx context.Context) (DesktopInstallation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return getDesktopInstallation(ctx, db.DB)
}

func GetDesktopInstallationWithExecutor(ctx context.Context, exec bun.IDB) (DesktopInstallation, bool, error) {
	return getDesktopInstallation(ctx, exec)
}

func getDesktopInstallation(ctx context.Context, exec bun.IDB) (DesktopInstallation, bool, error) {
	var installation DesktopInstallation
	err := exec.NewSelect().Model(&installation).Where("id = 1").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return DesktopInstallation{}, false, nil
	}
	if err != nil {
		return DesktopInstallation{}, false, err
	}
	return installation, true, nil
}

func InsertDesktopInstallationWithExecutor(ctx context.Context, exec bun.IDB, accountID, orgID int64) (DesktopInstallation, error) {
	now := time.Now()
	installation := DesktopInstallation{
		ID:        1,
		AccountID: accountID,
		OrgID:     orgID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := exec.NewInsert().Model(&installation).Exec(ctx)
	if err != nil {
		return DesktopInstallation{}, err
	}
	return installation, nil
}

// IsDesktopBootstrapPristineWithExecutor reports whether the identity and
// workspace tables are empty. Desktop bootstrap only adopts a fresh database;
// it never guesses which existing server identity should become the local user.
func IsDesktopBootstrapPristineWithExecutor(ctx context.Context, exec bun.IDB) (bool, error) {
	models := []any{
		(*Account)(nil),
		(*Organization)(nil),
		(*Workspace)(nil),
		(*InstanceAdmin)(nil),
	}
	for _, model := range models {
		count, err := exec.NewSelect().Model(model).Count(ctx)
		if err != nil {
			return false, err
		}
		if count != 0 {
			return false, nil
		}
	}
	return true, nil
}
