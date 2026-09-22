package settings_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/settings"
)

type fakeStore struct {
	instance      database.InstanceSettings
	instanceFound bool
	instanceErr   error

	overrides      database.OrganizationRuntimeSettings
	overridesFound bool
	overridesErr   error

	hasAdmin    bool
	hasAdminErr error

	initializedBaseURL string
	initializeCalls    int
	initializeErr      error
}

func (s *fakeStore) GetInstanceSettings(context.Context) (database.InstanceSettings, bool, error) {
	return s.instance, s.instanceFound, s.instanceErr
}

func (s *fakeStore) GetOrganizationRuntimeSettings(_ context.Context, _ int64) (database.OrganizationRuntimeSettings, bool, error) {
	return s.overrides, s.overridesFound, s.overridesErr
}

func (s *fakeStore) HasAnyInstanceAdmin(context.Context) (bool, error) {
	return s.hasAdmin, s.hasAdminErr
}

func (s *fakeStore) InitializeInstanceBaseURL(_ context.Context, baseURL string) (database.InstanceSettings, error) {
	s.initializeCalls++
	s.initializedBaseURL = baseURL
	if s.initializeErr != nil {
		return database.InstanceSettings{}, s.initializeErr
	}
	s.instance.BaseURL = baseURL
	return s.instance, nil
}

func validInstanceSettings() database.InstanceSettings {
	instance := database.DefaultInstanceSettings()
	instance.BaseURL = "https://sqlwarden.example.com"
	return instance
}

func storeWithInstance(instance database.InstanceSettings) *fakeStore {
	return &fakeStore{instance: instance, instanceFound: true}
}

func ptr[T any](value T) *T {
	return &value
}

func TestInstanceRejectsMissingRow(t *testing.T) {
	t.Parallel()
	service := settings.New(&fakeStore{})

	_, err := service.Instance(context.Background())
	if !errors.Is(err, settings.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestInstanceRejectsInvalidRow(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.InstanceName = ""
	service := settings.New(storeWithInstance(instance))

	_, err := service.Instance(context.Background())
	if !errors.Is(err, settings.ErrUnavailable) || !strings.Contains(err.Error(), "instance_name") {
		t.Fatalf("error = %v, want an unavailable error naming instance_name", err)
	}
}

func TestInstanceReportsStoreFailureAsUnavailable(t *testing.T) {
	t.Parallel()
	service := settings.New(&fakeStore{instanceErr: errors.New("database is closed")})

	_, err := service.Instance(context.Background())
	if !errors.Is(err, settings.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestEffectiveForOrgWithoutOrgReturnsInstanceValues(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	service := settings.New(storeWithInstance(instance))

	effective, err := service.EffectiveForOrg(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryMaxResultRows != instance.QueryMaxResultRows {
		t.Fatalf("query max result rows = %d, want %d", effective.QueryMaxResultRows, instance.QueryMaxResultRows)
	}
	if effective.JWTAccessTokenTTL != time.Duration(instance.JWTAccessTokenTTLSeconds)*time.Second {
		t.Fatalf("access token ttl = %s, want %ds", effective.JWTAccessTokenTTL, instance.JWTAccessTokenTTLSeconds)
	}
	if effective.SchemaSnapshotFreshness != time.Duration(instance.SchemaSnapshotFreshnessSeconds)*time.Second {
		t.Fatalf("snapshot freshness = %s, want %ds", effective.SchemaSnapshotFreshness, instance.SchemaSnapshotFreshnessSeconds)
	}
}

func TestEffectiveForOrgOverrides(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.QueryMaxResultRows = 1000
	instance.QueryMaxResultBytes = 1024
	instance.ExportsSyncMaxBytes = 4096
	instance.ExportsBackgroundMaxBytes = 0
	instance.SchemaSnapshotFreshnessSeconds = 3600
	instance.FileRevisionsEnabled = true
	instance.FileRevisionsKeepLatest = 10
	instance.QueryHistoryRetentionCount = 100

	tests := []struct {
		name      string
		overrides database.OrganizationRuntimeSettings
		check     func(*testing.T, settings.Effective)
	}{
		{
			name:      "narrowing row limit applies",
			overrides: database.OrganizationRuntimeSettings{QueryMaxResultRows: ptr(100)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryMaxResultRows != 100 {
					t.Fatalf("query max result rows = %d, want 100", effective.QueryMaxResultRows)
				}
			},
		},
		{
			name:      "loosening row limit is ignored",
			overrides: database.OrganizationRuntimeSettings{QueryMaxResultRows: ptr(100000)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryMaxResultRows != 1000 {
					t.Fatalf("query max result rows = %d, want the instance limit 1000", effective.QueryMaxResultRows)
				}
			},
		},
		{
			name:      "loosening byte limit is ignored",
			overrides: database.OrganizationRuntimeSettings{QueryMaxResultBytes: ptr(int64(1 << 30))},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryMaxResultBytes != 1024 {
					t.Fatalf("query max result bytes = %d, want the instance limit 1024", effective.QueryMaxResultBytes)
				}
			},
		},
		{
			name:      "unlimited background exports accept any override",
			overrides: database.OrganizationRuntimeSettings{ExportsBackgroundMaxBytes: ptr(int64(8192))},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.ExportsBackgroundMaxBytes != 8192 {
					t.Fatalf("exports background max bytes = %d, want 8192", effective.ExportsBackgroundMaxBytes)
				}
			},
		},
		{
			name:      "longer snapshot freshness applies because it demands less of the target",
			overrides: database.OrganizationRuntimeSettings{SchemaSnapshotFreshnessSeconds: ptr(int64(7200))},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.SchemaSnapshotFreshness != 2*time.Hour {
					t.Fatalf("snapshot freshness = %s, want 2h", effective.SchemaSnapshotFreshness)
				}
			},
		},
		{
			name:      "shorter snapshot freshness is ignored",
			overrides: database.OrganizationRuntimeSettings{SchemaSnapshotFreshnessSeconds: ptr(int64(60))},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.SchemaSnapshotFreshness != time.Hour {
					t.Fatalf("snapshot freshness = %s, want the instance value 1h", effective.SchemaSnapshotFreshness)
				}
			},
		},
		{
			name:      "file revisions can only be disabled",
			overrides: database.OrganizationRuntimeSettings{FileRevisionsEnabled: ptr(false)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.FileRevisionsEnabled {
					t.Fatal("file revisions should be disabled by the org override")
				}
			},
		},
		{
			name:      "file revision retention only narrows",
			overrides: database.OrganizationRuntimeSettings{FileRevisionsKeepLatest: ptr(99)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.FileRevisionsKeepLatest != 10 {
					t.Fatalf("file revisions keep latest = %d, want the instance value 10", effective.FileRevisionsKeepLatest)
				}
			},
		},
		{
			name:      "history retention only narrows",
			overrides: database.OrganizationRuntimeSettings{QueryHistoryRetentionCount: ptr(1000)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryHistoryRetentionCount != 100 {
					t.Fatalf("history retention = %d, want the instance value 100", effective.QueryHistoryRetentionCount)
				}
			},
		},
		{
			name:      "history mode override applies when the instance allows history",
			overrides: database.OrganizationRuntimeSettings{QueryHistoryMode: ptr(settings.HistoryModeLocal)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryHistoryMode != settings.HistoryModeLocal {
					t.Fatalf("history mode = %q, want local", effective.QueryHistoryMode)
				}
			},
		},
		{
			name:      "favorites mode override applies when the instance allows favorites",
			overrides: database.OrganizationRuntimeSettings{QueryFavoritesMode: ptr(settings.HistoryModeLocal)},
			check: func(t *testing.T, effective settings.Effective) {
				if effective.QueryFavoritesMode != settings.HistoryModeLocal {
					t.Fatalf("favorites mode = %q, want local", effective.QueryFavoritesMode)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := storeWithInstance(instance)
			store.overrides = test.overrides
			store.overridesFound = true

			effective, err := settings.New(store).EffectiveForOrg(context.Background(), ptr(int64(7)))
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, effective)
		})
	}
}

func TestEffectiveForOrgCannotReenableHistoryTurnedOffByTheInstance(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.QueryHistoryMode = settings.HistoryModeOff
	instance.QueryFavoritesMode = settings.HistoryModeOff
	store := storeWithInstance(instance)
	store.overrides = database.OrganizationRuntimeSettings{
		QueryHistoryMode:   ptr(settings.HistoryModeBackend),
		QueryFavoritesMode: ptr(settings.HistoryModeBackend),
	}
	store.overridesFound = true

	effective, err := settings.New(store).EffectiveForOrg(context.Background(), ptr(int64(1)))
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryHistoryMode != settings.HistoryModeOff {
		t.Fatalf("history mode = %q, want off", effective.QueryHistoryMode)
	}
	if effective.QueryFavoritesMode != settings.HistoryModeOff {
		t.Fatalf("favorites mode = %q, want off", effective.QueryFavoritesMode)
	}
}

func TestEffectiveForWorkspaceUsesOwningOrganization(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.QueryMaxResultRows = 1000
	store := storeWithInstance(instance)
	store.overrides = database.OrganizationRuntimeSettings{QueryMaxResultRows: ptr(25)}
	store.overridesFound = true
	service := settings.New(store)

	orgOwned, err := service.EffectiveForWorkspace(context.Background(), database.Workspace{OwnerType: "org", OwnerID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if orgOwned.QueryMaxResultRows != 25 {
		t.Fatalf("org workspace rows = %d, want the org override 25", orgOwned.QueryMaxResultRows)
	}

	personal, err := service.EffectiveForWorkspace(context.Background(), database.Workspace{OwnerType: "account", OwnerID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if personal.QueryMaxResultRows != 1000 {
		t.Fatalf("personal workspace rows = %d, want the instance value 1000", personal.QueryMaxResultRows)
	}
}

func TestPersonalSpacesEnabledReadsTheInstanceRow(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.PersonalSpacesEnabled = true

	enabled, err := settings.New(storeWithInstance(instance)).PersonalSpacesEnabled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("personal spaces should be enabled")
	}
}
