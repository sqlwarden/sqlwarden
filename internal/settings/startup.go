package settings

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/database"
)

// StartupStore is the metadata the startup helpers read and seed.
type StartupStore interface {
	InstanceSettingsStore
	HasAnyInstanceAdmin(ctx context.Context) (bool, error)
	InitializeInstanceBaseURL(ctx context.Context, baseURL string) (database.InstanceSettings, error)
}

// InitializeInstanceBaseURL seeds the stored instance base URL from
// bootstrapBaseURL until an instance admin exists or a base URL is stored.
// After that the database value is authoritative and only validated.
func InitializeInstanceBaseURL(ctx context.Context, store StartupStore, bootstrapBaseURL string) error {
	current, found, err := store.GetInstanceSettings(ctx)
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	if !found {
		return fmt.Errorf("initialize instance base URL: instance_settings row id=1 is missing; run database migrations")
	}
	if strings.TrimSpace(current.BaseURL) != "" {
		return Validate(Instance(current))
	}
	configured, err := store.HasAnyInstanceAdmin(ctx)
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	if configured {
		return Validate(Instance(current))
	}

	seeded, err := store.InitializeInstanceBaseURL(ctx, strings.TrimSpace(bootstrapBaseURL))
	if err != nil {
		return fmt.Errorf("initialize instance base URL: %w", err)
	}
	return Validate(Instance(seeded))
}

// ValidateInvariant fails startup when the stored instance settings row is
// missing or invalid.
func ValidateInvariant(ctx context.Context, store InstanceSettingsStore) error {
	current, found, err := store.GetInstanceSettings(ctx)
	if err != nil {
		return fmt.Errorf("validate runtime settings: %w", err)
	}
	if !found {
		return fmt.Errorf("validate runtime settings: instance_settings row id=1 is missing; run database migrations")
	}
	return Validate(Instance(current))
}
