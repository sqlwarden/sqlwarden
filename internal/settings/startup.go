package settings

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/database"
)

// BootstrapStore is the persistence contract startup seeding needs in addition
// to reading. *database.DB satisfies it.
type BootstrapStore interface {
	Store
	HasAnyInstanceAdmin(ctx context.Context) (bool, error)
	InitializeInstanceBaseURL(ctx context.Context, baseURL string) (database.InstanceSettings, error)
}

// Prepare seeds the instance base URL from bootstrap configuration when the
// instance has not been configured yet, then validates the resulting row.
//
// The composition root runs it after migrations and before any service is
// constructed, because every service that reads runtime settings assumes the
// row exists and is valid.
//
// Bootstrap configuration seeds the base URL only once: an instance that
// already has a base URL, or that has an instance admin, is database-owned from
// then on and a changed bootstrap value is ignored.
func Prepare(ctx context.Context, store BootstrapStore, bootstrapBaseURL string) error {
	current, found, err := store.GetInstanceSettings(ctx)
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	if !found {
		return fmt.Errorf("initialize instance base URL: instance_settings row id=1 is missing; run database migrations")
	}
	if strings.TrimSpace(current.BaseURL) != "" {
		return Validate(current)
	}
	configured, err := store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	if configured {
		return Validate(current)
	}

	seeded, err := store.InitializeInstanceBaseURL(ctx, strings.TrimSpace(bootstrapBaseURL))
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	return Validate(seeded)
}
