package settings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sqlwarden/internal/database"
)

// ErrUnavailable wraps every failure to read validated settings, whether the
// row is missing, unreadable, or invalid. Callers that must degrade rather than
// fail hard match on it with errors.Is.
var ErrUnavailable = errors.New("runtime settings unavailable")

// Store is the persistence contract the service reads settings through.
// *database.DB satisfies it.
type Store interface {
	GetInstanceSettings(ctx context.Context) (database.InstanceSettings, bool, error)
	GetOrganizationRuntimeSettings(ctx context.Context, orgID int64) (database.OrganizationRuntimeSettings, bool, error)
}

// Effective is the resolved operational settings that apply to a request: the
// instance values after any organization override has narrowed them. Durations
// are already converted, so callers never handle raw second counts.
type Effective struct {
	BaseURL                    string
	JWTAccessTokenTTL          time.Duration
	SessionsRevocationEnabled  bool
	QueryMaxResultRows         int
	QueryCursorPageSize        int
	QueryMaxResultBytes        int64
	ExportsSyncMaxBytes        int64
	ExportsBackgroundMaxBytes  int64
	SchemaSnapshotFreshness    time.Duration
	SchemaLazyThreshold        int
	FileRevisionsEnabled       bool
	FileRevisionsKeepLatest    int
	ErrorNotificationEmail     string
	QueryHistoryMode           string
	QueryHistoryRetentionCount int
	QueryFavoritesMode         string
}

// Service reads validated instance settings and resolves effective settings for
// an organization or workspace.
type Service struct {
	store Store
}

// New returns a service reading through store. A nil store yields a service
// whose reads all fail with [ErrUnavailable].
func New(store Store) *Service {
	return &Service{store: store}
}

// Instance returns the instance settings row after validating it. A row that
// fails [Validate] is reported as unavailable rather than served, because
// serving it would let an operator edit the instance into a state no caller can
// reason about.
func (s *Service) Instance(ctx context.Context) (database.InstanceSettings, error) {
	if s == nil || s.store == nil {
		return database.InstanceSettings{}, ErrUnavailable
	}
	settings, found, err := s.store.GetInstanceSettings(ctx)
	if err != nil {
		return database.InstanceSettings{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !found {
		return database.InstanceSettings{}, fmt.Errorf("%w: instance settings row is missing", ErrUnavailable)
	}
	if err := Validate(settings); err != nil {
		return database.InstanceSettings{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return settings, nil
}

// PersonalSpacesEnabled reports whether the instance allows personal spaces.
func (s *Service) PersonalSpacesEnabled(ctx context.Context) (bool, error) {
	settings, err := s.Instance(ctx)
	if err != nil {
		return false, err
	}
	return settings.PersonalSpacesEnabled, nil
}

// EffectiveForOrg resolves the settings that apply to work owned by orgID. A nil
// orgID resolves instance settings alone, which is the correct answer for
// personal-space and instance-scoped work.
func (s *Service) EffectiveForOrg(ctx context.Context, orgID *int64) (Effective, error) {
	instance, err := s.Instance(ctx)
	if err != nil {
		return Effective{}, err
	}
	effective := EffectiveFromInstance(instance)
	if orgID == nil {
		return effective, nil
	}
	overrides, found, err := s.store.GetOrganizationRuntimeSettings(ctx, *orgID)
	if err != nil {
		return Effective{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !found {
		return effective, nil
	}
	return applyOrgOverrides(effective, overrides), nil
}

// EffectiveForWorkspace resolves the settings that apply to work in workspace,
// using its owning organization when it has one.
func (s *Service) EffectiveForWorkspace(ctx context.Context, workspace database.Workspace) (Effective, error) {
	if workspace.OwnerType == "org" {
		orgID := workspace.OwnerID
		return s.EffectiveForOrg(ctx, &orgID)
	}
	return s.EffectiveForOrg(ctx, nil)
}

// EffectiveFromInstance projects an instance settings row onto the effective
// settings that apply when no organization override exists.
func EffectiveFromInstance(settings database.InstanceSettings) Effective {
	return Effective{
		BaseURL:                    settings.BaseURL,
		JWTAccessTokenTTL:          time.Duration(settings.JWTAccessTokenTTLSeconds) * time.Second,
		SessionsRevocationEnabled:  settings.SessionsRevocationEnabled,
		QueryMaxResultRows:         settings.QueryMaxResultRows,
		QueryCursorPageSize:        settings.QueryCursorPageSize,
		QueryMaxResultBytes:        settings.QueryMaxResultBytes,
		ExportsSyncMaxBytes:        settings.ExportsSyncMaxBytes,
		ExportsBackgroundMaxBytes:  settings.ExportsBackgroundMaxBytes,
		SchemaSnapshotFreshness:    time.Duration(settings.SchemaSnapshotFreshnessSeconds) * time.Second,
		SchemaLazyThreshold:        settings.SchemaLazyThreshold,
		FileRevisionsEnabled:       settings.FileRevisionsEnabled,
		FileRevisionsKeepLatest:    settings.FileRevisionsKeepLatest,
		ErrorNotificationEmail:     settings.ErrorNotificationEmail,
		QueryHistoryMode:           settings.QueryHistoryMode,
		QueryHistoryRetentionCount: settings.QueryHistoryRetentionCount,
		QueryFavoritesMode:         settings.QueryFavoritesMode,
	}
}

// applyOrgOverrides narrows effective settings with an organization's
// overrides. An override that would loosen an instance limit is ignored, except
// for schema snapshot freshness, where a longer interval is the weaker demand
// on the target database.
func applyOrgOverrides(effective Effective, overrides database.OrganizationRuntimeSettings) Effective {
	if overrides.QueryMaxResultRows != nil {
		if *overrides.QueryMaxResultRows > 0 && *overrides.QueryMaxResultRows < effective.QueryMaxResultRows {
			effective.QueryMaxResultRows = *overrides.QueryMaxResultRows
		}
	}
	if overrides.QueryMaxResultBytes != nil {
		if *overrides.QueryMaxResultBytes > 0 && *overrides.QueryMaxResultBytes < effective.QueryMaxResultBytes {
			effective.QueryMaxResultBytes = *overrides.QueryMaxResultBytes
		}
	}
	if overrides.ExportsSyncMaxBytes != nil {
		if *overrides.ExportsSyncMaxBytes > 0 && *overrides.ExportsSyncMaxBytes < effective.ExportsSyncMaxBytes {
			effective.ExportsSyncMaxBytes = *overrides.ExportsSyncMaxBytes
		}
	}
	if overrides.ExportsBackgroundMaxBytes != nil {
		if effective.ExportsBackgroundMaxBytes == 0 {
			if *overrides.ExportsBackgroundMaxBytes >= 0 {
				effective.ExportsBackgroundMaxBytes = *overrides.ExportsBackgroundMaxBytes
			}
		} else if *overrides.ExportsBackgroundMaxBytes > 0 && *overrides.ExportsBackgroundMaxBytes < effective.ExportsBackgroundMaxBytes {
			effective.ExportsBackgroundMaxBytes = *overrides.ExportsBackgroundMaxBytes
		}
	}
	if overrides.SchemaSnapshotFreshnessSeconds != nil {
		override := time.Duration(*overrides.SchemaSnapshotFreshnessSeconds) * time.Second
		if override > effective.SchemaSnapshotFreshness {
			effective.SchemaSnapshotFreshness = override
		}
	}
	if overrides.FileRevisionsEnabled != nil {
		effective.FileRevisionsEnabled = effective.FileRevisionsEnabled && *overrides.FileRevisionsEnabled
	}
	if overrides.FileRevisionsKeepLatest != nil {
		if *overrides.FileRevisionsKeepLatest >= 0 && *overrides.FileRevisionsKeepLatest < effective.FileRevisionsKeepLatest {
			effective.FileRevisionsKeepLatest = *overrides.FileRevisionsKeepLatest
		}
	}
	if effective.QueryHistoryMode != HistoryModeOff && overrides.QueryHistoryMode != nil {
		effective.QueryHistoryMode = *overrides.QueryHistoryMode
	}
	if overrides.QueryHistoryRetentionCount != nil && *overrides.QueryHistoryRetentionCount >= 1 && *overrides.QueryHistoryRetentionCount <= effective.QueryHistoryRetentionCount {
		effective.QueryHistoryRetentionCount = *overrides.QueryHistoryRetentionCount
	}
	if effective.QueryFavoritesMode != HistoryModeOff && overrides.QueryFavoritesMode != nil {
		effective.QueryFavoritesMode = *overrides.QueryFavoritesMode
	}
	return effective
}
