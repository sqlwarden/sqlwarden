package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/sqlwarden/internal/engine"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
)

// TargetPolicy decides which target databases may be registered on this
// instance.
//
// Driver registration alone is not sufficient: some registered drivers, such
// as SQLite, address host-local resources, so the instance settings gate them
// separately for file and in-memory targets.
type TargetPolicy struct {
	settings InstanceSettingsReader
}

// NewTargetPolicy returns the target policy reading instance settings through
// reader.
func NewTargetPolicy(reader InstanceSettingsReader) *TargetPolicy {
	return &TargetPolicy{settings: reader}
}

// Validate reports whether a target with this driver and DSN may be
// registered. It returns the driver registration error joined with
// [execution.ErrTargetRejected] for an unknown driver,
// and [ErrSQLiteTargetDisabled] or [ErrSQLiteInMemoryTargetDisabled] for a
// target the instance has turned off.
func (p *TargetPolicy) Validate(ctx context.Context, driverName, dsn string) error {
	driverName = strings.TrimSpace(driverName)
	dsn = strings.TrimSpace(dsn)

	if _, err := engine.New(driverName); err != nil {
		return errors.Join(execution.ErrTargetRejected, err)
	}
	if driverName != string(engine.DialectSQLite) {
		return nil
	}

	settings, err := p.settings.Instance(ctx)
	if err != nil {
		return err
	}
	if isInMemorySQLiteDSN(dsn) {
		if !settings.SQLiteInMemoryTargetsEnabled {
			return ErrSQLiteInMemoryTargetDisabled
		}
		return nil
	}
	if !settings.SQLiteLocalTargetsEnabled {
		return ErrSQLiteTargetDisabled
	}
	return nil
}

func isInMemorySQLiteDSN(dsn string) bool {
	return dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:")
}

// DriverSupportsSystemSchemas reports whether a driver's schema spec flags
// system scopes. It is the only case where a connection's "show system
// schemas" setting has any effect, so the catalog stores false for drivers
// that cannot honor it rather than persisting a setting that does nothing.
func DriverSupportsSystemSchemas(driverName string) bool {
	d, err := engine.New(driverName)
	if err != nil {
		return false
	}
	inspector, ok := d.(metadata.SchemaInspector)
	return ok && inspector.SchemaSpec().SystemSchemas
}
