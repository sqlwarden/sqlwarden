package catalog

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// Connection access modes.
const (
	AccessModeOpen       = "open"
	AccessModeRestricted = "restricted"
)

// ListConnectionsQuery filters a workspace's connections. EnvironmentID is set
// when the request addressed one environment, either through the route or
// through a query parameter that the caller has already parsed.
type ListConnectionsQuery struct {
	ListQuery
	EnvironmentID *int64
	Driver        string
	AccessMode    string
}

// ListConnections returns the workspace connections the account may see. The
// page is sorted newest-first by default, matching the order the underlying
// read established before the catalog owned this use case.
func (s *Service) ListConnections(ctx context.Context, accountID, orgID, workspaceID int64, query ListConnectionsQuery) (response.Paginated[database.Connection], error) {
	conns, err := s.store.AccessibleConnections(ctx, accountID, orgID, workspaceID)
	if err != nil {
		return response.Paginated[database.Connection]{}, err
	}
	return paginateConnections(filterConnections(conns, query), query.ListQuery), nil
}

// ListWorkspaceConnections returns every connection in an owner-scoped
// workspace. Ownership is established by the route before this use case runs.
func (s *Service) ListWorkspaceConnections(ctx context.Context, workspaceID int64, query ListConnectionsQuery) (response.Paginated[database.Connection], error) {
	return s.store.WorkspaceConnections(ctx, database.ListConnectionsParams{
		WorkspaceID: workspaceID, EnvironmentID: query.EnvironmentID,
		Search: query.Search, Driver: query.Driver, AccessMode: query.AccessMode,
		Sort: query.Sort, Order: query.Order, Page: query.Page, PageSize: query.PageSize,
	})
}

// Connection returns a connection only when the account may reach it under an
// organization-owned workspace.
func (s *Service) Connection(ctx context.Context, accountID, orgID int64, ws database.Workspace, conn database.Connection) (database.Connection, error) {
	if ws.OwnerType != "org" {
		return conn, nil
	}
	ok, err := s.store.HasAccessibleConnection(ctx, accountID, orgID, ws.ID, conn.ID)
	if err != nil {
		return database.Connection{}, err
	}
	if !ok {
		return database.Connection{}, ErrNotFound
	}
	return conn, nil
}

// RevealConnectionDSN returns a connection's plaintext DSN so an edit form can
// pre-fill it. An organization that masks credentials on edit refuses the
// reveal with [ErrCredentialsMasked], and the reveal is audited because it is
// the one catalog read that exposes a secret.
func (s *Service) RevealConnectionDSN(ctx context.Context, actor Actor, org database.Organization, conn database.Connection) (string, error) {
	if org.MaskConnectionCredentialsOnEdit {
		return "", s.auditDenied(ctx, actor, ActionConnectionDSNRevealed, resourceConnection, conn.ID,
			map[string]string{"reason": "masked_on_edit"}, ErrCredentialsMasked)
	}
	dsn, err := s.sealer.Decrypt(conn.DSNEncrypted)
	if err != nil {
		return "", err
	}
	if err := s.auditSuccess(ctx, actor, ActionConnectionDSNRevealed, resourceConnection, conn.ID, nil); err != nil {
		return "", err
	}
	return dsn, nil
}

// CreateConnectionInput is a request to register a target database in a
// workspace.
//
// DSN is plaintext and is sealed by the catalog before it is stored. SealedTLS
// and SealedSSH are already-sealed configuration documents: their wire schema
// and driver-aware validation belong to the transport that defines them, and
// the catalog stores the ciphertext unchanged. An empty value means the
// connection has no such configuration.
type CreateConnectionInput struct {
	Name              string
	Driver            string
	DSN               string
	EnvironmentID     *int64
	AccessMode        string
	DefaultScope      metadata.ScopePath
	ShowSystemSchemas bool
	SealedTLS         string
	SealedSSH         string
}

// ValidateCreateConnection records the rules a new connection must satisfy.
// The target check consults instance policy, so it needs a context.
func (s *Service) ValidateCreateConnection(ctx context.Context, input CreateConnectionInput, v *validator.Validator) {
	v.CheckField(input.Name != "", "name", "Name is required.")
	v.CheckField(input.Driver != "", "driver", "Driver is required.")
	v.CheckField(input.DSN != "", "dsn", "DSN is required.")

	if input.Driver != "" {
		if err := s.targets.Validate(ctx, input.Driver, input.DSN); err != nil {
			v.CheckField(false, "driver", TargetFieldMessage(err))
		}
	}

	accessMode := input.AccessMode
	if accessMode == "" {
		accessMode = AccessModeOpen
	}
	v.CheckField(accessMode == AccessModeOpen || accessMode == AccessModeRestricted,
		"access_mode", "Access mode must be open or restricted.")
}

// CreateConnection registers a target database in a workspace.
//
// The environment is proved to belong to the addressed workspace before the
// connection is written, so a connection can never be filed under another
// workspace's environment. The DSN is sealed here; only ciphertext reaches the
// store.
func (s *Service) CreateConnection(ctx context.Context, actor Actor, workspaceID int64, input CreateConnectionInput) (database.Connection, error) {
	var v validator.Validator
	s.ValidateCreateConnection(ctx, input, &v)
	if err := newValidationError(v); err != nil {
		return database.Connection{}, err
	}
	if input.AccessMode == "" {
		input.AccessMode = AccessModeOpen
	}

	environmentID, err := s.resolveConnectionEnvironment(ctx, workspaceID, input.EnvironmentID)
	if err != nil {
		return database.Connection{}, err
	}

	dsnEncrypted, err := s.sealer.Encrypt(input.DSN)
	if err != nil {
		return database.Connection{}, err
	}

	conn, err := s.store.CreateConnection(ctx, workspaceID, environmentID,
		input.Name, input.Driver, dsnEncrypted, input.AccessMode,
		input.DefaultScope, input.ShowSystemSchemas && DriverSupportsSystemSchemas(input.Driver))
	if err != nil {
		return database.Connection{}, err
	}

	if input.SealedTLS != "" {
		if err := s.store.UpdateConnectionTLSConfig(ctx, conn.ID, input.SealedTLS); err != nil {
			return database.Connection{}, err
		}
		conn.TLSConfigEncrypted = input.SealedTLS
	}
	if input.SealedSSH != "" {
		if err := s.store.UpdateConnectionSSHConfig(ctx, conn.ID, input.SealedSSH); err != nil {
			return database.Connection{}, err
		}
		conn.SSHConfigEncrypted = input.SealedSSH
	}

	if err := s.auditSuccess(ctx, actor, ActionConnectionCreated, resourceConnection, conn.ID, map[string]string{
		"workspace_id": strconv.FormatInt(workspaceID, 10),
		"driver":       conn.Driver,
		"access_mode":  conn.AccessMode,
		"tls":          strconv.FormatBool(input.SealedTLS != ""),
		"ssh":          strconv.FormatBool(input.SealedSSH != ""),
	}); err != nil {
		return database.Connection{}, err
	}
	return conn, nil
}

// UpdateConnectionInput carries the connection settings a request asked to
// change. A nil field is left as it is.
//
// SealedTLS and SealedSSH follow the same rule as on [CreateConnectionInput]:
// the transport owns the document schema, including merging a partial document
// with the stored one, and hands the catalog the sealed result.
type UpdateConnectionInput struct {
	Name                 *string
	DSN                  *string
	AccessMode           *string
	SchemaSnapshotPolicy *string
	DefaultScope         *metadata.ScopePath
	ShowSystemSchemas    *bool
	SealedTLS            *string
	SealedSSH            *string
	// Force authorizes dropping the live sessions a DSN or default-scope
	// change would otherwise invalidate.
	Force bool
}

// UpdateConnectionResult reports what a connection update changed, so the
// caller can run the schema-cache and snapshot follow-ups that belong to it.
type UpdateConnectionResult struct {
	Connection database.Connection
	// DSNRotated reports that the stored DSN changed.
	DSNRotated bool
	// ScopeChanged reports that the default scope changed, which makes cached
	// schema and completion metadata for this connection stale.
	ScopeChanged bool
	// SnapshotsDisabled reports that the schema snapshot policy went to
	// disabled, so scheduled snapshot work must be stopped and purged.
	SnapshotsDisabled bool
	// DroppedSessions is how many live sessions a forced change dropped.
	DroppedSessions int
}

// ValidateUpdateConnection records the rules a connection change must satisfy.
func ValidateUpdateConnection(input UpdateConnectionInput, v *validator.Validator) {
	if input.Name != nil {
		v.CheckField(trimmed(*input.Name) != "", "name", "Name must not be empty.")
	}
	if input.DSN != nil {
		v.CheckField(trimmed(*input.DSN) != "", "dsn", "DSN must not be empty.")
	}
	if input.AccessMode != nil {
		v.CheckField(*input.AccessMode == AccessModeOpen || *input.AccessMode == AccessModeRestricted,
			"access_mode", "Access mode must be open or restricted.")
	}
	if input.SchemaSnapshotPolicy != nil {
		v.CheckField(*input.SchemaSnapshotPolicy == database.SchemaSnapshotPolicyInherit ||
			*input.SchemaSnapshotPolicy == database.SchemaSnapshotPolicyDisabled,
			"schema_snapshot_policy", "Schema snapshot policy must be inherit or disabled.")
	}
	v.CheckField(input.Name != nil || input.DSN != nil || input.AccessMode != nil ||
		input.SchemaSnapshotPolicy != nil || input.DefaultScope != nil || input.ShowSystemSchemas != nil ||
		input.SealedTLS != nil || input.SealedSSH != nil,
		"request", "At least one setting is required.")
}

// UpdateConnection applies connection settings.
//
// A change that invalidates live sessions — rotating the DSN, or moving the
// default scope — is refused with [ErrActiveSessions] unless the caller set
// Force, because the alternative is silently serving queries through a session
// opened against the previous target or scope.
func (s *Service) UpdateConnection(ctx context.Context, actor Actor, conn database.Connection, input UpdateConnectionInput) (UpdateConnectionResult, error) {
	var v validator.Validator
	ValidateUpdateConnection(input, &v)
	if err := newValidationError(v); err != nil {
		return UpdateConnectionResult{}, err
	}

	currentDSN, err := s.sealer.Decrypt(conn.DSNEncrypted)
	if err != nil {
		return UpdateConnectionResult{}, err
	}
	nextDSN := currentDSN
	if input.DSN != nil {
		nextDSN = *input.DSN
	}
	if err := s.targets.Validate(ctx, conn.Driver, nextDSN); err != nil {
		var targetErrors validator.Validator
		targetErrors.AddFieldError("driver", TargetFieldMessage(err))
		return UpdateConnectionResult{}, &ValidationError{Validator: targetErrors}
	}

	dsnEncrypted := conn.DSNEncrypted
	if input.DSN != nil {
		dsnEncrypted, err = s.sealer.Encrypt(nextDSN)
		if err != nil {
			return UpdateConnectionResult{}, &SealError{Err: err}
		}
	}

	next := nextConnectionState(conn, input)
	result := UpdateConnectionResult{
		DSNRotated:   currentDSN != nextDSN,
		ScopeChanged: next.defaultScope != conn.DefaultScope,
	}

	if result.DSNRotated || result.ScopeChanged {
		reason := ActiveSessionsScopeChange
		if result.DSNRotated {
			reason = ActiveSessionsDSNRotation
		}
		connectionID := strconv.FormatInt(conn.ID, 10)
		active := s.sessions.CountForConnection(connectionID)
		if active > 0 {
			if !input.Force {
				return UpdateConnectionResult{}, s.auditDenied(ctx, actor, ActionConnectionUpdated, resourceConnection, conn.ID,
					map[string]string{"reason": string(reason), "active_sessions": strconv.Itoa(active)},
					&ActiveSessionsError{Reason: reason, Sessions: active})
			}
			s.sessions.RemoveForConnection(connectionID)
			result.DroppedSessions = active
		}
	}

	if err := s.store.UpdateConnection(ctx, conn.ID, next.name, dsnEncrypted, next.accessMode,
		next.snapshotPolicy, next.defaultScope, next.showSystemSchemas); err != nil {
		return UpdateConnectionResult{}, err
	}
	result.SnapshotsDisabled = conn.SchemaSnapshotPolicy != database.SchemaSnapshotPolicyDisabled &&
		next.snapshotPolicy == database.SchemaSnapshotPolicyDisabled

	if input.SealedTLS != nil {
		if err := s.store.UpdateConnectionTLSConfig(ctx, conn.ID, *input.SealedTLS); err != nil {
			return UpdateConnectionResult{}, err
		}
		conn.TLSConfigEncrypted = *input.SealedTLS
	}
	if input.SealedSSH != nil {
		if err := s.store.UpdateConnectionSSHConfig(ctx, conn.ID, *input.SealedSSH); err != nil {
			return UpdateConnectionResult{}, err
		}
		conn.SSHConfigEncrypted = *input.SealedSSH
	}

	conn.Name = next.name
	conn.DSNEncrypted = dsnEncrypted
	conn.AccessMode = next.accessMode
	conn.SchemaSnapshotPolicy = next.snapshotPolicy
	conn.DefaultScope = next.defaultScope
	conn.ShowSystemSchemas = next.showSystemSchemas
	result.Connection = conn

	if err := s.auditSuccess(ctx, actor, ActionConnectionUpdated, resourceConnection, conn.ID, map[string]string{
		"dsn_rotated":            strconv.FormatBool(result.DSNRotated),
		"scope_changed":          strconv.FormatBool(result.ScopeChanged),
		"access_mode":            next.accessMode,
		"schema_snapshot_policy": next.snapshotPolicy,
		"dropped_sessions":       strconv.Itoa(result.DroppedSessions),
	}); err != nil {
		return UpdateConnectionResult{}, err
	}
	return result, nil
}

// DeleteConnection removes a connection and invalidates the ancestry cached
// for it.
func (s *Service) DeleteConnection(ctx context.Context, actor Actor, conn database.Connection) error {
	if err := s.store.DeleteConnection(ctx, conn.ID); err != nil {
		return err
	}
	s.grants.InvalidateAncestry(resourceConnection, conn.ID)
	return s.auditSuccess(ctx, actor, ActionConnectionDeleted, resourceConnection, conn.ID, map[string]string{
		"workspace_id": strconv.FormatInt(conn.WorkspaceID, 10),
		"driver":       conn.Driver,
	})
}

// connectionState is the settled value of every connection setting an update
// may change, after nil fields have fallen back to what is stored.
type connectionState struct {
	name              string
	accessMode        string
	snapshotPolicy    string
	defaultScope      metadata.ScopePath
	showSystemSchemas bool
}

func nextConnectionState(conn database.Connection, input UpdateConnectionInput) connectionState {
	next := connectionState{
		name:              conn.Name,
		accessMode:        conn.AccessMode,
		snapshotPolicy:    conn.SchemaSnapshotPolicy,
		defaultScope:      conn.DefaultScope,
		showSystemSchemas: conn.ShowSystemSchemas,
	}
	if next.snapshotPolicy == "" {
		next.snapshotPolicy = database.SchemaSnapshotPolicyInherit
	}
	if input.Name != nil {
		next.name = *input.Name
	}
	if input.AccessMode != nil {
		next.accessMode = *input.AccessMode
	}
	if input.SchemaSnapshotPolicy != nil {
		next.snapshotPolicy = *input.SchemaSnapshotPolicy
	}
	if input.DefaultScope != nil {
		next.defaultScope = *input.DefaultScope
	}
	if input.ShowSystemSchemas != nil {
		next.showSystemSchemas = *input.ShowSystemSchemas
	}
	next.showSystemSchemas = next.showSystemSchemas && DriverSupportsSystemSchemas(conn.Driver)
	return next
}

// TargetFieldMessage renders a target policy refusal as the driver field
// message a caller sees.
func TargetFieldMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSQLiteTargetDisabled):
		return "SQLite file connections are disabled for this instance."
	case errors.Is(err, ErrSQLiteInMemoryTargetDisabled):
		return "In-memory SQLite connections are disabled for this instance."
	default:
		return "Driver must be a supported driver."
	}
}

func filterConnections(conns []database.Connection, query ListConnectionsQuery) []database.Connection {
	filtered := make([]database.Connection, 0, len(conns))
	search := strings.ToLower(trimmed(query.Search))

	for _, conn := range conns {
		if !matchesSearch(conn.Name, search) {
			continue
		}
		if query.EnvironmentID != nil && conn.EnvironmentID != *query.EnvironmentID {
			continue
		}
		if query.Driver != "" && conn.Driver != query.Driver {
			continue
		}
		if query.AccessMode != "" && conn.AccessMode != query.AccessMode {
			continue
		}
		filtered = append(filtered, conn)
	}

	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareConnection(filtered[i], filtered[j], query.Sort)
		if query.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
	return filtered
}

func paginateConnections(filtered []database.Connection, query ListQuery) response.Paginated[database.Connection] {
	total := len(filtered)
	start := min((query.Page-1)*query.PageSize, total)
	end := min(start+query.PageSize, total)
	return response.Paginated[database.Connection]{
		Items:    filtered[start:end],
		Page:     query.Page,
		PageSize: query.PageSize,
		Total:    total,
	}
}

func compareConnection(left, right database.Connection, sortBy string) int {
	switch sortBy {
	case "name":
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}
	case "driver":
		if left.Driver != right.Driver {
			return strings.Compare(left.Driver, right.Driver)
		}
	default:
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.Before(right.CreatedAt) {
				return -1
			}
			return 1
		}
	}
	return compareIDs(left.ID, right.ID)
}
