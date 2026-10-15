// Package settings holds policies derived from the stored instance settings.
package settings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
)

// ErrTargetDenied is wrapped by every TargetPolicy rejection, so callers can
// tell a policy decision apart from a failure to reach the target.
var ErrTargetDenied = errors.New("target connection denied")

var (
	ErrUnsupportedDriver            = fmt.Errorf("%w: unsupported driver", ErrTargetDenied)
	ErrSQLiteFileTargetDisabled     = fmt.Errorf("%w: sqlite file target connections are disabled for this instance", ErrTargetDenied)
	ErrSQLiteInMemoryTargetDisabled = fmt.Errorf("%w: sqlite in-memory target connections are disabled for this instance", ErrTargetDenied)
)

// InstanceSettingsStore reads the singleton instance settings row.
type InstanceSettingsStore interface {
	GetInstanceSettings(ctx context.Context) (database.InstanceSettings, bool, error)
}

// TargetPolicy enforces the server-side policy for user-supplied database
// targets. Driver registration alone is not enough because some registered
// drivers, such as SQLite, may expose host-local resources.
type TargetPolicy struct {
	store InstanceSettingsStore
}

func NewTargetPolicy(store InstanceSettingsStore) *TargetPolicy {
	return &TargetPolicy{store: store}
}

// Check returns nil when the target may be connected to. Every rejection
// wraps ErrTargetDenied; settings that cannot be read deny the target.
func (p *TargetPolicy) Check(ctx context.Context, driver, dsn string) error {
	driver = strings.TrimSpace(driver)
	dsn = strings.TrimSpace(dsn)

	if _, err := engine.New(driver); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedDriver, err)
	}
	if driver != string(engine.DialectSQLite) {
		return nil
	}

	if p.store == nil {
		return fmt.Errorf("%w: instance settings unavailable", ErrTargetDenied)
	}
	settings, found, err := p.store.GetInstanceSettings(ctx)
	if err != nil {
		return fmt.Errorf("%w: instance settings unavailable: %v", ErrTargetDenied, err)
	}
	if !found {
		return fmt.Errorf("%w: instance settings row is missing", ErrTargetDenied)
	}
	if isInMemorySQLiteDSN(dsn) {
		if !settings.SQLiteInMemoryTargetsEnabled {
			return ErrSQLiteInMemoryTargetDisabled
		}
		return nil
	}
	if !settings.SQLiteLocalTargetsEnabled {
		return ErrSQLiteFileTargetDisabled
	}
	return nil
}

func isInMemorySQLiteDSN(dsn string) bool {
	return dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:")
}
