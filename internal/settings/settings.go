package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/platform/invalidate"
)

// ErrUnavailable is wrapped when the stored instance settings cannot be read
// or are invalid. Callers fail closed instead of falling back to defaults.
var ErrUnavailable = errors.New("runtime settings unavailable")

// Instance is the stored singleton instance settings row.
type Instance database.InstanceSettings

// OrgOverrides are an organization's stored runtime overrides. A nil field
// inherits the instance value.
type OrgOverrides database.OrganizationRuntimeSettings

// Effective are the runtime settings in force for one scope: the instance
// values narrowed by the organization's overrides where it has any.
type Effective struct {
	BaseURL                    string
	JWTAccessTokenTTL          time.Duration
	SessionsRevocationEnabled  bool
	QueryMaxResultRows         int
	QueryCursorPageSize        int
	QueryMaxResultBytes        int64
	ExportsSyncMaxBytes        int64
	ExportsBackgroundMaxBytes  int64
	FileRevisionsEnabled       bool
	FileRevisionsKeepLatest    int
	ErrorNotificationEmail     string
	QueryHistoryMode           string
	QueryHistoryRetentionCount int
	QueryFavoritesMode         string
}

// Store is the metadata the settings service reads and writes.
type Store interface {
	GetInstanceSettings(ctx context.Context) (database.InstanceSettings, bool, error)
	UpsertInstanceSettings(ctx context.Context, settings database.InstanceSettings) (database.InstanceSettings, error)
	GetOrganizationRuntimeSettings(ctx context.Context, orgID int64) (database.OrganizationRuntimeSettings, bool, error)
	UpsertOrganizationRuntimeSettings(ctx context.Context, settings database.OrganizationRuntimeSettings) (database.OrganizationRuntimeSettings, error)
	ClearQueryHistoryForOrg(ctx context.Context, orgID int64) error
	ClearQueryFavoritesForOrg(ctx context.Context, orgID int64) error
	QueryHistoryHasRowsForOrg(ctx context.Context, orgID int64) (bool, error)
	QueryFavoritesHasRowsForOrg(ctx context.Context, orgID int64) (bool, error)
}

// Sealer encrypts instance secrets, such as the SMTP password, before they
// are stored.
type Sealer interface {
	Encrypt(plaintext string) (string, error)
}

// InstanceKey is the key published on invalidate.TopicInstanceSettings. The
// instance settings are a singleton row, so every change uses the same key.
const InstanceKey = "instance"

type Deps struct {
	Store    Store
	Requirer *access.Requirer
	Sealer   Sealer
	// FileStorageMode is the configured workspace file storage mode. File
	// revisions cannot be enabled in file mode.
	FileStorageMode string
	// Invalidator, when set, is told after every stored instance settings
	// change so settings watchers in every process apply it without waiting
	// for their poll.
	Invalidator invalidate.Invalidator
	Logger      *slog.Logger
}

// Service reads and updates instance and organization runtime settings.
type Service struct {
	store           Store
	requirer        *access.Requirer
	sealer          Sealer
	fileStorageMode string
	inval           invalidate.Invalidator
	logger          *slog.Logger
}

func New(deps Deps) *Service {
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store:           deps.Store,
		requirer:        deps.Requirer,
		sealer:          deps.Sealer,
		fileStorageMode: deps.FileStorageMode,
		inval:           deps.Invalidator,
		logger:          logger,
	}
}

// publishInstanceChange announces a stored instance settings change. A
// failed publish never fails the update: watchers still pick the change up
// on their next poll.
func (s *Service) publishInstanceChange(ctx context.Context) {
	if s.inval == nil {
		return
	}
	if err := s.inval.Publish(ctx, invalidate.TopicInstanceSettings, InstanceKey); err != nil {
		s.logger.WarnContext(ctx, "instance settings invalidation publish failed", "error", err)
	}
}

// Instance returns the stored instance settings. A missing or invalid row is
// reported as ErrUnavailable.
func (s *Service) Instance(ctx context.Context) (Instance, error) {
	if s == nil || s.store == nil {
		return Instance{}, ErrUnavailable
	}
	row, found, err := s.store.GetInstanceSettings(ctx)
	if err != nil {
		return Instance{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !found {
		return Instance{}, fmt.Errorf("%w: instance settings row is missing", ErrUnavailable)
	}
	in := Instance(row)
	if err := Validate(in); err != nil {
		return Instance{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return in, nil
}

// EffectiveForOrg returns the instance settings narrowed by the
// organization's overrides. A nil orgID returns the instance settings.
func (s *Service) EffectiveForOrg(ctx context.Context, orgID *int64) (Effective, error) {
	in, err := s.Instance(ctx)
	if err != nil {
		return Effective{}, err
	}
	effective := EffectiveFromInstance(in)
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
	return applyOrgOverrides(effective, OrgOverrides(overrides)), nil
}

// EffectiveForWorkspace applies the owning organization's overrides for
// org-owned workspaces and the instance settings alone otherwise.
func (s *Service) EffectiveForWorkspace(ctx context.Context, ws access.Workspace) (Effective, error) {
	var orgID *int64
	if ws.OwnerType == "org" {
		id := ws.OwnerID
		orgID = &id
	}
	return s.EffectiveForOrg(ctx, orgID)
}

// EffectiveFromInstance returns the effective settings of a scope without
// organization overrides.
func EffectiveFromInstance(in Instance) Effective {
	return Effective{
		BaseURL:                    in.BaseURL,
		JWTAccessTokenTTL:          time.Duration(in.JWTAccessTokenTTLSeconds) * time.Second,
		SessionsRevocationEnabled:  in.SessionsRevocationEnabled,
		QueryMaxResultRows:         in.QueryMaxResultRows,
		QueryCursorPageSize:        in.QueryCursorPageSize,
		QueryMaxResultBytes:        in.QueryMaxResultBytes,
		ExportsSyncMaxBytes:        in.ExportsSyncMaxBytes,
		ExportsBackgroundMaxBytes:  in.ExportsBackgroundMaxBytes,
		FileRevisionsEnabled:       in.FileRevisionsEnabled,
		FileRevisionsKeepLatest:    in.FileRevisionsKeepLatest,
		ErrorNotificationEmail:     in.ErrorNotificationEmail,
		QueryHistoryMode:           in.QueryHistoryMode,
		QueryHistoryRetentionCount: in.QueryHistoryRetentionCount,
		QueryFavoritesMode:         in.QueryFavoritesMode,
	}
}

// applyOrgOverrides lets an organization only narrow the instance settings:
// limits shrink, features turn off, and a mode the instance turned off stays
// off. Stored overrides that would widen a limit are ignored, because the
// instance limit may have shrunk after the override was saved.
func applyOrgOverrides(effective Effective, overrides OrgOverrides) Effective {
	if v := overrides.QueryMaxResultRows; v != nil && *v > 0 && *v < effective.QueryMaxResultRows {
		effective.QueryMaxResultRows = *v
	}
	if v := overrides.QueryMaxResultBytes; v != nil && *v > 0 && *v < effective.QueryMaxResultBytes {
		effective.QueryMaxResultBytes = *v
	}
	if v := overrides.ExportsSyncMaxBytes; v != nil && *v > 0 && *v < effective.ExportsSyncMaxBytes {
		effective.ExportsSyncMaxBytes = *v
	}
	if v := overrides.ExportsBackgroundMaxBytes; v != nil {
		// 0 means unlimited, so any non-negative override narrows an
		// unlimited instance, while a limited instance never becomes unlimited.
		if effective.ExportsBackgroundMaxBytes == 0 {
			if *v >= 0 {
				effective.ExportsBackgroundMaxBytes = *v
			}
		} else if *v > 0 && *v < effective.ExportsBackgroundMaxBytes {
			effective.ExportsBackgroundMaxBytes = *v
		}
	}
	if v := overrides.FileRevisionsEnabled; v != nil {
		effective.FileRevisionsEnabled = effective.FileRevisionsEnabled && *v
	}
	if v := overrides.FileRevisionsKeepLatest; v != nil && *v >= 0 && *v < effective.FileRevisionsKeepLatest {
		effective.FileRevisionsKeepLatest = *v
	}
	if effective.QueryHistoryMode != HistoryModeOff && overrides.QueryHistoryMode != nil {
		effective.QueryHistoryMode = *overrides.QueryHistoryMode
	}
	if v := overrides.QueryHistoryRetentionCount; v != nil && *v >= 1 && *v <= effective.QueryHistoryRetentionCount {
		effective.QueryHistoryRetentionCount = *v
	}
	if effective.QueryFavoritesMode != HistoryModeOff && overrides.QueryFavoritesMode != nil {
		effective.QueryFavoritesMode = *overrides.QueryFavoritesMode
	}
	return effective
}

// requireInstanceAdmin admits account principals the authentication chain
// flagged as instance admins.
func requireInstanceAdmin(p access.Principal) error {
	if p.Subject.Kind != access.SubjectAccount || !p.InstanceAdmin {
		return access.ErrForbidden
	}
	return nil
}

// requireOrg admits org members who hold permission on the organization.
func (s *Service) requireOrg(ctx context.Context, p access.Principal, orgID int64, permission string) error {
	if _, err := s.requirer.Org(ctx, p, orgID); err != nil {
		return err
	}
	if !s.requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
		return access.ErrForbidden
	}
	return nil
}
