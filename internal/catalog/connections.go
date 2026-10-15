package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
	schemaapp "github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/internal/settings"
	"golang.org/x/crypto/ssh"
)

const connectionTestTimeout = 10 * time.Second

var allSecretNames = [...]credentials.SecretName{
	credentials.SecretPassword,
	credentials.SecretSSHPassword,
	credentials.SecretSSHPrivateKey,
	credentials.SecretSSHPassphrase,
	credentials.SecretTLSClientKey,
}

// ConnRef identifies a connection and the routed hierarchy used to reach it.
// EnvironmentID is optional for workspace-level connection routes.
type ConnRef struct {
	OrgID         int64
	WorkspaceID   int64
	EnvironmentID *int64
	ConnectionID  int64
}

type TLSConfig struct {
	Mode          string `json:"mode"`
	ServerName    string `json:"server_name,omitempty"`
	CAPEM         string `json:"ca_pem,omitempty"`
	ClientCertPEM string `json:"client_cert_pem,omitempty"`
}

func (c TLSConfig) empty() bool {
	return strings.TrimSpace(c.Mode) == "" && c.ServerName == "" && c.CAPEM == "" && c.ClientCertPEM == ""
}

type SSHConfig struct {
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host,omitempty"`
	Port                int    `json:"port,omitempty"`
	User                string `json:"user,omitempty"`
	AuthMethod          string `json:"auth_method,omitempty"`
	KnownHostsEntry     string `json:"known_hosts_entry,omitempty"`
	Fingerprint         string `json:"fingerprint,omitempty"`
	InsecureSkipHostKey bool   `json:"insecure_skip_host_key,omitempty"`
}

func (c SSHConfig) empty() bool {
	return !c.Enabled && c.Host == "" && c.Port == 0 && c.User == "" && c.AuthMethod == "" &&
		c.KnownHostsEntry == "" && c.Fingerprint == "" && !c.InsecureSkipHostKey
}

type SecretView struct {
	Set        bool   `json:"set"`
	Source     string `json:"source,omitempty"`
	Revealable bool   `json:"revealable"`
}

type ConnectionView struct {
	ID                   int64                                 `json:"id"`
	WorkspaceID          int64                                 `json:"workspace_id"`
	EnvironmentID        int64                                 `json:"environment_id"`
	Name                 string                                `json:"name"`
	Driver               string                                `json:"driver"`
	Params               engine.Params                         `json:"params"`
	TLSConfig            *TLSConfig                            `json:"tls_config,omitempty"`
	SSHConfig            *SSHConfig                            `json:"ssh_config,omitempty"`
	Secrets              map[credentials.SecretName]SecretView `json:"secrets"`
	AccessMode           string                                `json:"access_mode"`
	SchemaSnapshotPolicy string                                `json:"schema_snapshot_policy"`
	DefaultScope         metadata.ScopePath                    `json:"default_scope,omitempty"`
	ShowSystemSchemas    bool                                  `json:"show_system_schemas"`
	ShowAllDatabases     bool                                  `json:"show_all_databases"`
	CreatedAt            time.Time                             `json:"created_at"`
	UpdatedAt            time.Time                             `json:"updated_at"`
}

type CreateInput struct {
	OrgID                      int64
	WorkspaceID                int64
	EnvironmentID              *int64
	AuthorizationEnvironmentID *int64
	Name                       string
	Driver                     string
	Params                     engine.Params
	TLSConfig                  *TLSConfig
	SSHConfig                  *SSHConfig
	Secrets                    map[credentials.SecretName]*string
	AccessMode                 string
	DefaultScope               metadata.ScopePath
	ShowSystemSchemas          bool
	ShowAllDatabases           bool
}

type UpdateInput struct {
	Name                 *string
	Params               engine.Params
	TLSConfig            *TLSConfig
	SSHConfig            *SSHConfig
	Secrets              map[credentials.SecretName]*string
	AccessMode           *string
	SchemaSnapshotPolicy *string
	DefaultScope         *metadata.ScopePath
	ShowSystemSchemas    *bool
	ShowAllDatabases     *bool
	Force                bool
}

type TestInput struct {
	OrgID                      int64
	WorkspaceID                int64
	EnvironmentID              *int64
	AuthorizationEnvironmentID *int64
	ConnectionID               *int64
	Driver                     string
	Params                     engine.Params
	TLSConfig                  *TLSConfig
	SSHConfig                  *SSHConfig
	Secrets                    map[credentials.SecretName]*string
	ParentScope                metadata.ScopePath
	Limits                     execution.Limits
}

type TestResult struct {
	OK                  bool                     `json:"ok"`
	LatencyMS           int64                    `json:"latency_ms"`
	Error               string                   `json:"error,omitempty"`
	ScopeDiscovery      *metadata.ScopeDiscovery `json:"scope_discovery,omitempty"`
	ScopeDiscoveryError string                   `json:"scope_discovery_error,omitempty"`

	// Stage and ErrorCategory describe a failed probe for logs and status
	// mapping; they are never serialized.
	Stage         string `json:"-"`
	ErrorCategory string `json:"-"`
}

const (
	TestStageDriverInit = "driver_init"
	TestStageSSHTunnel  = "ssh_tunnel"
	TestStageConnect    = "connect"
)

func testErrorCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, settings.ErrTargetDenied):
		return "policy_denied"
	case strings.Contains(err.Error(), "unknown driver"):
		return "unsupported_driver"
	default:
		return "target_unreachable"
	}
}

// redactSecrets removes the DSN and every supplied secret value from driver
// error text before it is returned to the caller, including the URL-escaped,
// quote-escaped and newline-collapsed forms drivers commonly echo.
func redactSecrets(text, dsn string, secrets map[string]string) string {
	seen := map[string]struct{}{}
	var values []string
	add := func(v string) {
		if v == "" {
			return
		}
		if _, dup := seen[v]; dup {
			return
		}
		seen[v] = struct{}{}
		values = append(values, v)
	}
	for _, raw := range append([]string{dsn}, mapValues(secrets)...) {
		add(raw)
		add(url.QueryEscape(raw))
		add(strings.ReplaceAll(url.QueryEscape(raw), "+", "%20"))
		add(url.PathEscape(raw))
		quoted := strconv.Quote(raw)
		add(quoted[1 : len(quoted)-1])
		add(strings.Join(strings.Fields(raw), " "))
		add(strings.Join(strings.Fields(raw), ""))
	}
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	for _, v := range values {
		text = strings.ReplaceAll(text, v, "[redacted]")
	}
	return text
}

// RedactConnectionError removes resolved connection material from an error
// returned while opening a stored connection.
func RedactConnectionError(text string, creds credentials.Credentials) string {
	secrets := map[string]string{}
	if spec, ok := engine.ConnectionSpecFor(creds.Driver); ok {
		if _, parsed, err := spec.ParseDSN(creds.DSN); err == nil {
			for name, value := range parsed {
				secrets[name] = value
			}
		}
	}
	if creds.TLS != nil {
		secrets[string(credentials.SecretTLSClientKey)] = creds.TLS.ClientKeyPEM
	}
	if creds.SSH != nil {
		secrets[string(credentials.SecretSSHPassword)] = creds.SSH.Password
		secrets[string(credentials.SecretSSHPrivateKey)] = creds.SSH.PrivateKeyPEM
		secrets[string(credentials.SecretSSHPassphrase)] = creds.SSH.Passphrase
	}
	return redactSecrets(text, creds.DSN, secrets)
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

type Store interface {
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	GetWorkspace(ctx context.Context, id int64) (database.Workspace, bool, error)
	GetEnvironment(ctx context.Context, id int64) (database.Environment, bool, error)
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
	InsertConnectionWithScope(ctx context.Context, workspaceID int64, envID *int64, name, driver, dsnEncrypted, accessMode string, defaultScope metadata.ScopePath, showSystemSchemas, showAllDatabases bool) (database.Connection, error)
	UpdateConnectionWithScopeAndPolicy(ctx context.Context, id int64, name, accessMode, snapshotPolicy string, defaultScope metadata.ScopePath, showSystemSchemas, showAllDatabases bool) error
	UpdateConnectionStructured(ctx context.Context, id int64, params, tlsConfig, sshConfig json.RawMessage) error
	DeleteConnection(ctx context.Context, id int64) error
	DeleteSchemaCache(ctx context.Context, connectionID int64) error
}

type TargetPolicy interface {
	Check(ctx context.Context, driver, dsn string) error
}

type SessionRevoker interface {
	CountForConnection(ctx context.Context, connectionID string) (int, error)
	RevokeConnection(ctx context.Context, connectionID string) (int, error)
}

type AncestryInvalidator interface {
	InvalidateAncestry(resourceType string, resourceID int64)
}

type Deps struct {
	Store        Store
	Policy       access.PolicyEvaluator
	Credentials  credentials.Provider
	Writer       credentials.Writer
	RevealPolicy credentials.RevealPolicy
	Audit        audit.Writer
	Logger       *slog.Logger
	TargetPolicy TargetPolicy
	Prober       execution.Prober
	Revoker      SessionRevoker
	Ancestry     AncestryInvalidator
	SpecLookup   credentials.SpecLookup
}

type Service struct {
	deps Deps
}

func New(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(discardWriter{}, nil))
	}
	if deps.SpecLookup == nil {
		deps.SpecLookup = engine.ConnectionSpecFor
	}
	return &Service{deps: deps}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func (s *Service) Create(ctx context.Context, p access.Principal, in CreateInput) (ConnectionView, error) {
	ws, err := s.authorizeCreate(ctx, p, in.OrgID, in.WorkspaceID, in.AuthorizationEnvironmentID)
	if err != nil {
		return ConnectionView{}, err
	}
	if in.EnvironmentID != nil {
		if err := s.requireEnvironmentOwnership(ctx, in.WorkspaceID, *in.EnvironmentID); err != nil {
			return ConnectionView{}, err
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Driver = strings.TrimSpace(in.Driver)
	if in.Name == "" {
		return ConnectionView{}, validationError("name", "Name is required.")
	}
	if in.Driver == "" {
		return ConnectionView{}, validationError("driver", "Driver is required.")
	}
	if in.AccessMode == "" {
		in.AccessMode = "open"
	}
	if in.AccessMode != "open" && in.AccessMode != "restricted" {
		return ConnectionView{}, validationError("access_mode", "Access mode must be open or restricted.")
	}
	params, spec, err := s.validateParams(in.Driver, in.Params)
	if err != nil {
		return ConnectionView{}, err
	}
	plainSecrets, err := inputSecretValues(in.Secrets)
	if err != nil {
		return ConnectionView{}, err
	}
	if err := validateTLS(in.Driver, in.TLSConfig, plainSecrets); err != nil {
		return ConnectionView{}, err
	}
	if err := validateSSH(in.Driver, in.SSHConfig, plainSecrets); err != nil {
		return ConnectionView{}, err
	}
	dsn, err := spec.BuildDSN(params, engine.Secrets(plainSecrets))
	if err != nil {
		return ConnectionView{}, validationError("params", "Connection parameters are invalid.")
	}
	if s.deps.TargetPolicy != nil {
		if err := s.deps.TargetPolicy.Check(ctx, in.Driver, dsn); err != nil {
			return ConnectionView{}, err
		}
	}

	showSystemSchemas := in.ShowSystemSchemas && driverSupportsSystemSchemas(in.Driver)
	set, _ := engine.Describe(in.Driver)
	showAllDatabases := resolveShowAllDatabases(set.Tree, in.DefaultScope, in.ShowAllDatabases)
	tlsRaw, sshRaw, err := encodeConfigs(in.TLSConfig, in.SSHConfig)
	if err != nil {
		return ConnectionView{}, err
	}
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return ConnectionView{}, errors.New("catalog: encode connection parameters")
	}
	conn, err := s.deps.Store.InsertConnectionWithScope(ctx, ws.ID, in.EnvironmentID, in.Name, in.Driver, "", in.AccessMode, in.DefaultScope, showSystemSchemas, showAllDatabases)
	if err != nil {
		return ConnectionView{}, err
	}
	ref := ConnRef{OrgID: in.OrgID, WorkspaceID: in.WorkspaceID, EnvironmentID: in.EnvironmentID, ConnectionID: conn.ID}
	// The row, its structured columns and its secrets are written by separate
	// stores, so any failure after the insert removes the row (secrets cascade).
	abandon := func(cause error) (ConnectionView, error) {
		cleanupCtx := context.WithoutCancel(ctx)
		if err := s.deps.Store.DeleteConnection(cleanupCtx, conn.ID); err != nil {
			s.deps.Logger.Error("connection create cleanup failed", "connection_id", conn.ID)
		} else if s.deps.Ancestry != nil {
			s.deps.Ancestry.InvalidateAncestry("connection", conn.ID)
		}
		return ConnectionView{}, cause
	}
	if err := s.deps.Store.UpdateConnectionStructured(ctx, conn.ID, paramsRaw, tlsRaw, sshRaw); err != nil {
		return abandon(err)
	}
	if err := s.applySecrets(ctx, credentialsRef(ref), in.Secrets); err != nil {
		return abandon(err)
	}
	conn.Params, conn.TLSConfig, conn.SSHConfig = paramsRaw, tlsRaw, sshRaw
	view, err := s.connectionView(ctx, p, ref, ws, conn)
	if err != nil {
		return abandon(err)
	}
	s.deps.Logger.Info("connection created", "connection_id", conn.ID, "workspace_id", conn.WorkspaceID, "driver", conn.Driver)
	return view, nil
}

func (s *Service) Get(ctx context.Context, p access.Principal, ref ConnRef) (ConnectionView, error) {
	conn, ws, err := s.loadConnection(ctx, p, ref, access.PermConnRead)
	if err != nil {
		return ConnectionView{}, err
	}
	return s.connectionView(ctx, p, ref, ws, conn)
}

func (s *Service) Update(ctx context.Context, p access.Principal, ref ConnRef, in UpdateInput) error {
	conn, ws, err := s.loadConnection(ctx, p, ref, access.PermConnUpdate)
	if err != nil {
		return err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return validationError("name", "Name must not be empty.")
		}
		in.Name = &name
	}
	if in.AccessMode != nil && *in.AccessMode != "open" && *in.AccessMode != "restricted" {
		return validationError("access_mode", "Access mode must be open or restricted.")
	}
	if in.SchemaSnapshotPolicy != nil && *in.SchemaSnapshotPolicy != database.SchemaSnapshotPolicyInherit && *in.SchemaSnapshotPolicy != database.SchemaSnapshotPolicyDisabled {
		return validationError("schema_snapshot_policy", "Schema snapshot policy must be inherit or disabled.")
	}
	if in.Name == nil && in.Params == nil && in.TLSConfig == nil && in.SSHConfig == nil && in.Secrets == nil && in.AccessMode == nil && in.SchemaSnapshotPolicy == nil && in.DefaultScope == nil && in.ShowSystemSchemas == nil && in.ShowAllDatabases == nil {
		return validationError("request", "At least one setting is required.")
	}

	currentParams, currentTLS, currentSSH, err := decodeConfigs(conn)
	if err != nil {
		return err
	}
	nextParams := currentParams
	if in.Params != nil {
		nextParams = in.Params
	}
	nextParams, spec, err := s.validateParams(conn.Driver, nextParams)
	if err != nil {
		return err
	}
	nextTLS, nextSSH := currentTLS, currentSSH
	if in.TLSConfig != nil {
		nextTLS = in.TLSConfig
	}
	if in.SSHConfig != nil {
		nextSSH = in.SSHConfig
	}
	plainChanges, err := inputSecretValues(in.Secrets)
	if err != nil {
		return err
	}
	if err := s.rejectReferenceSecretChanges(ctx, credentialsRef(ref), in.Secrets); err != nil {
		return err
	}
	tlsInputsChanged := in.TLSConfig != nil || secretChanged(in.Secrets, credentials.SecretTLSClientKey)
	sshInputsChanged := in.SSHConfig != nil || secretChanged(in.Secrets, credentials.SecretSSHPassword) || secretChanged(in.Secrets, credentials.SecretSSHPrivateKey) || secretChanged(in.Secrets, credentials.SecretSSHPassphrase)
	baseline := currentParams
	if normalized, _, err := s.validateParams(conn.Driver, currentParams); err == nil {
		baseline = normalized
	}
	currentTarget := canonicalConnectionTarget(baseline, currentTLS, currentSSH)
	nextTarget := canonicalConnectionTarget(nextParams, nextTLS, nextSSH)
	paramsChanged := in.Params != nil && !equalParams(currentTarget.Params, nextTarget.Params)
	tlsConfigChanged := currentTarget.TLS != nextTarget.TLS
	sshConfigChanged := currentTarget.SSH != nextTarget.SSH
	if connectionTargetChanged(spec, currentTarget, nextTarget) {
		gate := &revealGate{service: s, principal: p, ref: ref, ownerType: ws.OwnerType}
		if err := s.requireSecretsResupplied(ctx, ref, in.Secrets, allSecretNames[:], gate); err != nil {
			return err
		}
	}
	if tlsInputsChanged || sshInputsChanged {
		validationSecrets, err := s.mergeStoredSecrets(ctx, credentialsRef(ref), in.Secrets, plainChanges)
		if err != nil {
			return err
		}
		if tlsInputsChanged {
			if err := validateTLS(conn.Driver, nextTLS, validationSecrets); err != nil {
				return err
			}
		}
		if sshInputsChanged {
			if err := validateSSH(conn.Driver, nextSSH, validationSecrets); err != nil {
				return err
			}
		}
	}
	dsn, err := spec.BuildDSN(nextParams, engine.Secrets(plainChanges))
	if err != nil {
		return validationError("params", "Connection parameters are invalid.")
	}
	if s.deps.TargetPolicy != nil {
		if err := s.deps.TargetPolicy.Check(ctx, conn.Driver, dsn); err != nil {
			return err
		}
	}

	passwordChanged := secretChanged(in.Secrets, credentials.SecretPassword)
	nextDefaultScope := conn.DefaultScope
	if in.DefaultScope != nil {
		nextDefaultScope = *in.DefaultScope
	}
	scopeChanged := nextDefaultScope != conn.DefaultScope
	connectionTargetChanged := paramsChanged || tlsConfigChanged || sshConfigChanged || passwordChanged
	dropSessions := false
	if connectionTargetChanged || scopeChanged {
		if s.deps.Revoker != nil {
			active, err := s.deps.Revoker.CountForConnection(ctx, strconv.FormatInt(conn.ID, 10))
			if err != nil {
				return err
			}
			if active > 0 && !in.Force {
				return ErrActiveSessions
			}
			dropSessions = active > 0 && in.Force
		}
	}
	if connectionTargetChanged {
		if err := s.deps.Store.DeleteSchemaCache(ctx, conn.ID); err != nil {
			return err
		}
	}

	nextName := conn.Name
	if in.Name != nil {
		nextName = *in.Name
	}
	nextAccessMode := conn.AccessMode
	if in.AccessMode != nil {
		nextAccessMode = *in.AccessMode
	}
	nextSnapshotPolicy := conn.SchemaSnapshotPolicy
	if nextSnapshotPolicy == "" {
		nextSnapshotPolicy = database.SchemaSnapshotPolicyInherit
	}
	if in.SchemaSnapshotPolicy != nil {
		nextSnapshotPolicy = *in.SchemaSnapshotPolicy
	}
	nextShowSystemSchemas := conn.ShowSystemSchemas
	if in.ShowSystemSchemas != nil {
		nextShowSystemSchemas = *in.ShowSystemSchemas
	}
	nextShowSystemSchemas = nextShowSystemSchemas && driverSupportsSystemSchemas(conn.Driver)
	nextShowAllDatabases := conn.ShowAllDatabases
	if in.ShowAllDatabases != nil {
		nextShowAllDatabases = *in.ShowAllDatabases
	}
	set, _ := engine.Describe(conn.Driver)
	nextShowAllDatabases = resolveShowAllDatabases(set.Tree, nextDefaultScope, nextShowAllDatabases)

	paramsRaw, err := json.Marshal(nextParams)
	if err != nil {
		return errors.New("catalog: encode connection parameters")
	}
	tlsRaw, sshRaw, err := encodeConfigs(nextTLS, nextSSH)
	if err != nil {
		return err
	}
	// Columns and secrets live in separate stores; a failure part-way restores
	// the original columns and secrets so no half-applied configuration remains.
	restoreColumns := func() {
		cleanupCtx := context.WithoutCancel(ctx)
		original := conn.SchemaSnapshotPolicy
		if original == "" {
			original = database.SchemaSnapshotPolicyInherit
		}
		err1 := s.deps.Store.UpdateConnectionWithScopeAndPolicy(cleanupCtx, conn.ID, conn.Name, conn.AccessMode, original, conn.DefaultScope, conn.ShowSystemSchemas, conn.ShowAllDatabases)
		err2 := s.deps.Store.UpdateConnectionStructured(cleanupCtx, conn.ID, conn.Params, conn.TLSConfig, conn.SSHConfig)
		if err1 != nil || err2 != nil {
			s.deps.Logger.Error("connection update rollback failed", "connection_id", conn.ID)
		}
	}
	if err := s.deps.Store.UpdateConnectionWithScopeAndPolicy(ctx, conn.ID, nextName, nextAccessMode, nextSnapshotPolicy, nextDefaultScope, nextShowSystemSchemas, nextShowAllDatabases); err != nil {
		restoreColumns()
		return err
	}
	if err := s.deps.Store.UpdateConnectionStructured(ctx, conn.ID, paramsRaw, tlsRaw, sshRaw); err != nil {
		restoreColumns()
		return err
	}
	if err := s.applySecretsWithRollback(ctx, credentialsRef(ref), in.Secrets); err != nil {
		restoreColumns()
		return err
	}
	if conn.SchemaSnapshotPolicy != database.SchemaSnapshotPolicyDisabled && nextSnapshotPolicy == database.SchemaSnapshotPolicyDisabled && !connectionTargetChanged {
		if err := s.deps.Store.DeleteSchemaCache(ctx, conn.ID); err != nil {
			return err
		}
	}
	if dropSessions && s.deps.Revoker != nil {
		if _, err := s.deps.Revoker.RevokeConnection(context.WithoutCancel(ctx), strconv.FormatInt(conn.ID, 10)); err != nil {
			return err
		}
	}
	s.deps.Logger.Info("connection updated", "connection_id", conn.ID, "target_changed", connectionTargetChanged, "scope_changed", scopeChanged)
	return nil
}

func (s *Service) Delete(ctx context.Context, p access.Principal, ref ConnRef) error {
	conn, _, err := s.loadConnection(ctx, p, ref, access.PermConnDelete)
	if err != nil {
		return err
	}
	if err := s.deps.Store.DeleteConnection(ctx, conn.ID); err != nil {
		return err
	}
	if s.deps.Ancestry != nil {
		s.deps.Ancestry.InvalidateAncestry("connection", conn.ID)
	}
	s.deps.Logger.Info("connection deleted", "connection_id", conn.ID, "workspace_id", conn.WorkspaceID, "driver", conn.Driver)
	return nil
}

func (s *Service) Test(ctx context.Context, p access.Principal, in TestInput) (TestResult, error) {
	_, err := s.authorizeCreate(ctx, p, in.OrgID, in.WorkspaceID, in.AuthorizationEnvironmentID)
	if err != nil {
		return TestResult{}, err
	}
	if in.EnvironmentID != nil {
		if err := s.requireEnvironmentOwnership(ctx, in.WorkspaceID, *in.EnvironmentID); err != nil {
			return TestResult{}, err
		}
	}
	var (
		storedRef  *ConnRef
		storedConn database.Connection
		storedWS   database.Workspace
	)
	if in.ConnectionID != nil {
		ref := ConnRef{OrgID: in.OrgID, WorkspaceID: in.WorkspaceID, EnvironmentID: in.EnvironmentID, ConnectionID: *in.ConnectionID}
		conn, ws, err := s.loadConnection(ctx, p, ref, access.PermConnRead)
		if err != nil {
			return TestResult{}, err
		}
		storedRef, storedConn, storedWS = &ref, conn, ws
		if in.Driver == "" {
			in.Driver = conn.Driver
		}
	}
	if driver := strings.TrimSpace(in.Driver); driver != "" {
		if _, found := s.deps.SpecLookup(driver); !found {
			if _, initErr := engine.New(driver); initErr != nil {
				return TestResult{Stage: TestStageDriverInit, ErrorCategory: "unsupported_driver", Error: initErr.Error()}, nil
			}
		}
	}
	params, spec, err := s.validateParams(in.Driver, in.Params)
	if err != nil {
		return TestResult{}, err
	}
	plainSecrets, err := inputSecretValues(in.Secrets)
	if err != nil {
		return TestResult{}, err
	}
	if storedRef != nil {
		gate := &revealGate{service: s, principal: p, ref: *storedRef, ownerType: storedWS.OwnerType}
		if err := s.fillStoredSecretsForTest(ctx, *storedRef, storedConn, in, params, spec, plainSecrets, gate); err != nil {
			return TestResult{}, err
		}
	}
	if err := validateTLS(in.Driver, in.TLSConfig, plainSecrets); err != nil {
		return TestResult{}, err
	}
	if err := validateSSH(in.Driver, in.SSHConfig, plainSecrets); err != nil {
		return TestResult{}, err
	}
	dsn, err := spec.BuildDSN(params, engine.Secrets(plainSecrets))
	if err != nil {
		return TestResult{}, validationError("params", "Connection parameters are invalid.")
	}
	if s.deps.TargetPolicy != nil {
		if err := s.deps.TargetPolicy.Check(ctx, in.Driver, dsn); err != nil {
			return TestResult{}, err
		}
	}
	if s.deps.Prober == nil {
		return TestResult{}, errors.New("catalog: connection prober is not configured")
	}

	testCtx, cancel := context.WithTimeout(ctx, connectionTestTimeout)
	defer cancel()
	start := time.Now()
	creds := credentials.Credentials{Driver: in.Driver, DSN: dsn, TLS: tlsToEngine(in.TLSConfig, plainSecrets), SSH: sshToEngine(in.SSHConfig, plainSecrets)}
	scope := execution.Scope{OrgID: strconv.FormatInt(in.OrgID, 10), WorkspaceID: strconv.FormatInt(in.WorkspaceID, 10), AccountID: strconv.FormatInt(p.Subject.ID, 10)}
	result := TestResult{OK: true}
	err = s.deps.Prober.Probe(testCtx, scope, creds, in.Limits, func(inspector metadata.SchemaInspector) error {
		result.LatencyMS = time.Since(start).Milliseconds()
		discovery, discoveryErr := schemaapp.DiscoverScopes(testCtx, inspector.Tree(), schemaapp.LiveFromInspector(inspector), in.ParentScope)
		if discoveryErr != nil {
			result.ScopeDiscoveryError = redactSecrets(discoveryErr.Error(), dsn, plainSecrets)
		} else {
			result.ScopeDiscovery = &discovery
		}
		return nil
	})
	if errors.Is(err, execution.ErrSchemaUnsupported) {
		result.LatencyMS = time.Since(start).Milliseconds()
		err = nil
	}
	if err != nil {
		result.OK = false
		result.LatencyMS = time.Since(start).Milliseconds()
		var (
			connectErr *execution.ConnectError
			tunnelErr  *execution.TunnelError
		)
		switch {
		case errors.As(err, &tunnelErr):
			result.Stage = TestStageSSHTunnel
			result.ErrorCategory = testErrorCategory(tunnelErr.Err)
			result.Error = "SSH tunnel: " + redactSecrets(tunnelErr.Err.Error(), dsn, plainSecrets)
		case errors.As(err, &connectErr):
			result.Stage = TestStageConnect
			result.ErrorCategory = testErrorCategory(err)
			result.Error = redactSecrets(err.Error(), dsn, plainSecrets)
		default:
			return TestResult{}, err
		}
		return result, nil
	}
	s.deps.Logger.Info("connection test completed", "driver", in.Driver, "latency_ms", result.LatencyMS, "ok", true)
	return result, nil
}

// fillStoredSecretsForTest reuses stored secrets the caller did not supply, but
// only when the tested target is identical to the stored one. Otherwise a caller
// holding conn:read could steer a stored secret to a host of their choosing,
// bypassing the reveal permission, policy and audit trail.
func (s *Service) fillStoredSecretsForTest(ctx context.Context, ref ConnRef, conn database.Connection, in TestInput, params engine.Params, spec engine.ConnectionSpec, plain map[string]string, gate *revealGate) error {
	if s.deps.Credentials == nil {
		return errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, credentialsRef(ref))
	if err != nil {
		return normalizeCredentialError(err)
	}
	sameTarget := s.sameStoredTarget(conn, in, params, spec)
	for _, name := range allSecretNames {
		if _, supplied := in.Secrets[name]; supplied || !states[name].Set {
			continue
		}
		field := "secrets." + string(name)
		if states[name].Source != credentials.SourceStored {
			return validationError(field, "This secret is managed by reference; enter it to test the connection.")
		}
		if !sameTarget {
			revealable, err := gate.revealable(ctx, states[name])
			if err != nil {
				return err
			}
			if !revealable {
				return validationError(field, "The connection target changed; re-enter this secret to test it.")
			}
		}
	}
	for _, name := range allSecretNames {
		if _, supplied := in.Secrets[name]; supplied || !states[name].Set {
			continue
		}
		value, err := s.deps.Credentials.Reveal(ctx, credentialsRef(ref), name)
		if err != nil {
			return normalizeCredentialError(err)
		}
		if !sameTarget {
			if err := s.emitSecretRevealedAudit(ctx, gate.principal, ref, name, "test"); err != nil {
				return err
			}
		}
		plain[string(name)] = value
	}
	return nil
}

func (s *Service) sameStoredTarget(conn database.Connection, in TestInput, params engine.Params, spec engine.ConnectionSpec) bool {
	if in.Driver != conn.Driver {
		return false
	}
	storedParams, storedTLS, storedSSH, err := decodeConfigs(conn)
	if err != nil {
		return false
	}
	storedParams, _, err = s.validateParams(conn.Driver, storedParams)
	if err != nil {
		return false
	}
	stored := canonicalConnectionTarget(storedParams, storedTLS, storedSSH)
	incoming := canonicalConnectionTarget(params, in.TLSConfig, in.SSHConfig)
	return !connectionTargetChanged(spec, stored, incoming)
}

type connectionTarget struct {
	Params engine.Params
	TLS    TLSConfig
	SSH    SSHConfig
}

func canonicalConnectionTarget(params engine.Params, tlsConfig *TLSConfig, sshConfig *SSHConfig) connectionTarget {
	normalizedParams := make(engine.Params, len(params))
	for key, value := range params {
		normalizedParams[key] = strings.TrimSpace(value)
	}
	tlsValue := tlsOrEmpty(tlsConfig)
	if tlsValue.Mode == "" {
		tlsValue.Mode = string(engine.TLSModeDisable)
	}
	tlsValue.ServerName = strings.TrimSpace(tlsValue.ServerName)
	sshValue := sshOrEmpty(sshConfig)
	sshValue.Host = strings.TrimSpace(sshValue.Host)
	sshValue.User = strings.TrimSpace(sshValue.User)
	sshValue.AuthMethod = strings.TrimSpace(sshValue.AuthMethod)
	if sshValue.Port == 0 {
		sshValue.Port = 22
	}
	if sshValue.AuthMethod == "" {
		sshValue.AuthMethod = string(credentials.SSHAuthPassword)
	}
	return connectionTarget{Params: normalizedParams, TLS: tlsValue, SSH: sshValue}
}

// databaseParamKey names the params field that selects a database on the same
// server; changing it does not move where stored secrets are sent.
const databaseParamKey = "database"

func networkParamsChanged(spec engine.ConnectionSpec, current, next engine.Params) bool {
	nonNetwork := map[string]bool{}
	for _, field := range spec.Fields() {
		if field.NonNetwork {
			nonNetwork[field.Key] = true
		}
	}
	if len(nonNetwork) == 0 {
		nonNetwork[databaseParamKey] = true
	}
	for _, params := range []engine.Params{current, next} {
		for key := range params {
			if !nonNetwork[key] && current[key] != next[key] {
				return true
			}
		}
	}
	return false
}

func connectionTargetChanged(spec engine.ConnectionSpec, current, next connectionTarget) bool {
	return networkParamsChanged(spec, current.Params, next.Params) || current.TLS != next.TLS || current.SSH != next.SSH
}

// requireSecretsResupplied rejects an update that moves the network target
// while leaving a stored secret untouched. Otherwise a caller holding
// conn:update but not conn:reveal_secret could repoint the connection at a
// host they control and receive the stored credential.
func (s *Service) requireSecretsResupplied(ctx context.Context, ref ConnRef, changes map[credentials.SecretName]*string, names []credentials.SecretName, gate *revealGate) error {
	if s.deps.Credentials == nil {
		return errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, credentialsRef(ref))
	if err != nil {
		return normalizeCredentialError(err)
	}
	for _, name := range names {
		if _, supplied := changes[name]; supplied {
			continue
		}
		state := states[name]
		if !state.Set {
			continue
		}
		if state.Source == credentials.SourceReference {
			return validationError("secrets."+string(name), "This secret is managed by reference; its connection target cannot be changed.")
		}
		revealable, err := gate.revealable(ctx, state)
		if err != nil {
			return err
		}
		if !revealable {
			return validationError("secrets."+string(name), "The connection target changed; re-enter this secret or clear it.")
		}
	}
	return nil
}

func tlsOrEmpty(c *TLSConfig) TLSConfig {
	if c == nil || c.empty() {
		return TLSConfig{}
	}
	value := *c
	value.Mode = strings.TrimSpace(value.Mode)
	return value
}

func sshOrEmpty(c *SSHConfig) SSHConfig {
	if c == nil || c.empty() {
		return SSHConfig{}
	}
	return *c
}

func (s *Service) authorizeCreate(ctx context.Context, p access.Principal, orgID, workspaceID int64, environmentID *int64) (database.Workspace, error) {
	ws, err := s.loadWorkspace(ctx, p, orgID, workspaceID)
	if err != nil {
		return database.Workspace{}, err
	}
	resourceType, resourceID := "workspace", workspaceID
	if environmentID != nil {
		if err := s.requireEnvironmentOwnership(ctx, workspaceID, *environmentID); err != nil {
			return database.Workspace{}, err
		}
		resourceType, resourceID = "environment", *environmentID
	}
	if s.deps.Policy == nil || !s.deps.Policy.Can(ctx, p.Subject.ID, orgID, ws.OwnerType, resourceType, resourceID, access.PermConnCreate) {
		return database.Workspace{}, ErrForbidden
	}
	return ws, nil
}

func (s *Service) loadWorkspace(ctx context.Context, p access.Principal, orgID, workspaceID int64) (database.Workspace, error) {
	if s.deps.Store == nil {
		return database.Workspace{}, errors.New("catalog: store is not configured")
	}
	if p.Subject.Kind != access.SubjectAccount {
		return database.Workspace{}, ErrForbidden
	}
	member, err := s.deps.Store.IsOrgMember(ctx, orgID, p.Subject.ID)
	if err != nil {
		return database.Workspace{}, err
	}
	if !member {
		return database.Workspace{}, ErrForbidden
	}
	ws, found, err := s.deps.Store.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return database.Workspace{}, err
	}
	if !found || ws.OwnerType != "org" || ws.OrgID == nil || *ws.OrgID != orgID {
		return database.Workspace{}, ErrNotFound
	}
	return ws, nil
}

func (s *Service) requireEnvironmentOwnership(ctx context.Context, workspaceID, environmentID int64) error {
	env, found, err := s.deps.Store.GetEnvironment(ctx, environmentID)
	if err != nil {
		return err
	}
	if !found || env.WorkspaceID != workspaceID {
		return ErrNotFound
	}
	return nil
}

func (s *Service) loadConnection(ctx context.Context, p access.Principal, ref ConnRef, permission string) (database.Connection, database.Workspace, error) {
	ws, err := s.loadWorkspace(ctx, p, ref.OrgID, ref.WorkspaceID)
	if err != nil {
		return database.Connection{}, database.Workspace{}, err
	}
	conn, found, err := s.deps.Store.GetConnection(ctx, ref.ConnectionID)
	if err != nil {
		return database.Connection{}, database.Workspace{}, err
	}
	if !found || conn.WorkspaceID != ref.WorkspaceID || (ref.EnvironmentID != nil && conn.EnvironmentID != *ref.EnvironmentID) {
		return database.Connection{}, database.Workspace{}, ErrNotFound
	}
	if s.deps.Policy == nil || !s.deps.Policy.Can(ctx, p.Subject.ID, ref.OrgID, ws.OwnerType, "connection", conn.ID, permission) {
		return database.Connection{}, database.Workspace{}, ErrForbidden
	}
	return conn, ws, nil
}

func (s *Service) validateParams(driver string, params engine.Params) (engine.Params, engine.ConnectionSpec, error) {
	spec, ok := s.deps.SpecLookup(strings.TrimSpace(driver))
	if !ok || spec == nil {
		return nil, nil, validationError("driver", "Driver is not supported.")
	}
	normalized := make(engine.Params, len(params))
	for key, value := range params {
		normalized[key] = strings.TrimSpace(value)
	}
	fields := spec.Fields()
	for key := range normalized {
		if !slices.ContainsFunc(fields, func(field engine.FieldSpec) bool { return !field.Secret && field.Key == key }) {
			return nil, nil, validationError("params", "Connection parameters contain an unknown field.")
		}
	}
	for _, field := range fields {
		if field.Secret {
			continue
		}
		if normalized[field.Key] == "" && field.Default != "" {
			normalized[field.Key] = field.Default
		}
		value := normalized[field.Key]
		if field.Required && value == "" {
			return nil, nil, validationError("params."+field.Key, field.Label+" is required.")
		}
		if value == "" {
			continue
		}
		switch field.Type {
		case engine.FieldTypeInt:
			if _, err := strconv.Atoi(value); err != nil {
				return nil, nil, validationError("params."+field.Key, field.Label+" must be an integer.")
			}
		case engine.FieldTypeBool:
			if _, err := strconv.ParseBool(value); err != nil {
				return nil, nil, validationError("params."+field.Key, field.Label+" must be true or false.")
			}
		case engine.FieldTypeEnum:
			if !slices.Contains(field.Options, value) {
				return nil, nil, validationError("params."+field.Key, field.Label+" has an unsupported value.")
			}
		}
	}
	return normalized, spec, nil
}

func (s *Service) applySecrets(ctx context.Context, ref credentials.ConnectionRef, changes map[credentials.SecretName]*string) error {
	if len(changes) == 0 {
		return nil
	}
	if s.deps.Writer == nil {
		return errors.New("catalog: credential writer is not configured")
	}
	for _, name := range allSecretNames {
		value, present := changes[name]
		if !present {
			continue
		}
		if value == nil {
			if err := s.deps.Writer.Clear(ctx, ref, name); err != nil {
				return normalizeCredentialError(err)
			}
			continue
		}
		if err := s.deps.Writer.Set(ctx, ref, name, *value); err != nil {
			return normalizeCredentialError(err)
		}
	}
	return nil
}

// rejectReferenceSecretChanges keeps externally managed secrets immutable so a
// failed update can never lose a reference binding that cannot be restored.
func (s *Service) rejectReferenceSecretChanges(ctx context.Context, ref credentials.ConnectionRef, changes map[credentials.SecretName]*string) error {
	if len(changes) == 0 {
		return nil
	}
	if s.deps.Credentials == nil {
		return errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, ref)
	if err != nil {
		return normalizeCredentialError(err)
	}
	for _, name := range allSecretNames {
		if _, changed := changes[name]; !changed {
			continue
		}
		if state := states[name]; state.Set && state.Source == credentials.SourceReference {
			return validationError("secrets."+string(name), "This secret is managed externally and cannot be changed here.")
		}
	}
	return nil
}

// applySecretsWithRollback applies changes and, if one fails, puts back the
// previous value of every secret already changed. Reference-sourced secrets
// are rejected before any write, so they never reach this path.
func (s *Service) applySecretsWithRollback(ctx context.Context, ref credentials.ConnectionRef, changes map[credentials.SecretName]*string) error {
	if len(changes) == 0 {
		return nil
	}
	if s.deps.Credentials == nil || s.deps.Writer == nil {
		return errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, ref)
	if err != nil {
		return normalizeCredentialError(err)
	}
	previous := make(map[credentials.SecretName]*string, len(changes))
	for _, name := range allSecretNames {
		if _, changed := changes[name]; !changed {
			continue
		}
		state := states[name]
		switch {
		case !state.Set:
			previous[name] = nil
		case state.Source == credentials.SourceStored:
			value, err := s.deps.Credentials.Reveal(ctx, ref, name)
			if err != nil {
				return normalizeCredentialError(err)
			}
			previous[name] = &value
		}
	}
	if err := s.applySecrets(ctx, ref, changes); err != nil {
		cleanupCtx := context.WithoutCancel(ctx)
		if restoreErr := s.applySecrets(cleanupCtx, ref, previous); restoreErr != nil {
			s.deps.Logger.Error("connection secret rollback failed", "connection_id", ref.ConnectionID)
		}
		return err
	}
	return nil
}

const referenceSecretMarker = "\x00managed-by-reference"

func (s *Service) mergeStoredSecrets(ctx context.Context, ref credentials.ConnectionRef, changes map[credentials.SecretName]*string, supplied map[string]string) (map[string]string, error) {
	merged := make(map[string]string, len(allSecretNames))
	for name, value := range supplied {
		merged[name] = value
	}
	if s.deps.Credentials == nil {
		return nil, errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, ref)
	if err != nil {
		return nil, normalizeCredentialError(err)
	}
	for _, name := range allSecretNames {
		if _, changed := changes[name]; changed {
			continue
		}
		state := states[name]
		if !state.Set {
			continue
		}
		if state.Source == credentials.SourceReference {
			merged[string(name)] = referenceSecretMarker
			continue
		}
		value, err := s.deps.Credentials.Reveal(ctx, ref, name)
		if err != nil {
			return nil, normalizeCredentialError(err)
		}
		merged[string(name)] = value
	}
	return merged, nil
}

func (s *Service) connectionView(ctx context.Context, p access.Principal, ref ConnRef, ws database.Workspace, conn database.Connection) (ConnectionView, error) {
	params, tlsConfig, sshConfig, err := decodeConfigs(conn)
	if err != nil {
		return ConnectionView{}, err
	}
	if s.deps.Credentials == nil {
		return ConnectionView{}, errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, credentialsRef(ref))
	if err != nil {
		return ConnectionView{}, normalizeCredentialError(err)
	}
	gate := &revealGate{service: s, principal: p, ref: ref, ownerType: ws.OwnerType}
	secretViews := make(map[credentials.SecretName]SecretView, len(allSecretNames))
	for _, name := range allSecretNames {
		state := states[name]
		revealable, err := gate.revealable(ctx, state)
		if err != nil {
			return ConnectionView{}, err
		}
		secretViews[name] = SecretView{Set: state.Set, Source: string(state.Source), Revealable: revealable}
	}
	return ConnectionView{
		ID: conn.ID, WorkspaceID: conn.WorkspaceID, EnvironmentID: conn.EnvironmentID,
		Name: conn.Name, Driver: conn.Driver, Params: params, TLSConfig: tlsConfig, SSHConfig: sshConfig,
		Secrets: secretViews, AccessMode: conn.AccessMode, SchemaSnapshotPolicy: conn.SchemaSnapshotPolicy,
		DefaultScope: conn.DefaultScope, ShowSystemSchemas: conn.ShowSystemSchemas, ShowAllDatabases: conn.ShowAllDatabases,
		CreatedAt: conn.CreatedAt, UpdatedAt: conn.UpdatedAt,
	}, nil
}

func credentialsRef(ref ConnRef) credentials.ConnectionRef {
	return credentials.ConnectionRef{OrgID: strconv.FormatInt(ref.OrgID, 10), WorkspaceID: strconv.FormatInt(ref.WorkspaceID, 10), ConnectionID: strconv.FormatInt(ref.ConnectionID, 10)}
}

func normalizeCredentialError(err error) error {
	if errors.Is(err, credentials.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func inputSecretValues(changes map[credentials.SecretName]*string) (map[string]string, error) {
	values := make(map[string]string, len(changes))
	for name, value := range changes {
		if !slices.Contains(allSecretNames[:], name) {
			return nil, validationError("secrets", "Secret name is not supported.")
		}
		if value != nil {
			if *value == "" {
				return nil, validationError("secrets."+string(name), "Secret values must not be empty; omit the secret to keep it or use null to clear it.")
			}
			values[string(name)] = *value
		}
	}
	return values, nil
}

func secretChanged(changes map[credentials.SecretName]*string, name credentials.SecretName) bool {
	_, ok := changes[name]
	return ok
}

func equalParams(a, b engine.Params) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func decodeConfigs(conn database.Connection) (engine.Params, *TLSConfig, *SSHConfig, error) {
	params := engine.Params{}
	if len(conn.Params) != 0 {
		if err := json.Unmarshal(conn.Params, &params); err != nil {
			return nil, nil, nil, fmt.Errorf("catalog: connection %d parameters are malformed", conn.ID)
		}
	}
	var tlsConfig *TLSConfig
	if len(conn.TLSConfig) != 0 {
		var value TLSConfig
		if err := json.Unmarshal(conn.TLSConfig, &value); err != nil {
			return nil, nil, nil, fmt.Errorf("catalog: connection %d tls config is malformed", conn.ID)
		}
		tlsConfig = &value
	}
	var sshConfig *SSHConfig
	if len(conn.SSHConfig) != 0 {
		var value SSHConfig
		if err := json.Unmarshal(conn.SSHConfig, &value); err != nil {
			return nil, nil, nil, fmt.Errorf("catalog: connection %d ssh config is malformed", conn.ID)
		}
		sshConfig = &value
	}
	return params, tlsConfig, sshConfig, nil
}

func encodeConfigs(tlsConfig *TLSConfig, sshConfig *SSHConfig) (json.RawMessage, json.RawMessage, error) {
	var tlsRaw, sshRaw json.RawMessage
	var err error
	if tlsConfig != nil && !tlsConfig.empty() {
		tlsRaw, err = json.Marshal(tlsConfig)
		if err != nil {
			return nil, nil, errors.New("catalog: encode tls config")
		}
	}
	if sshConfig != nil && !sshConfig.empty() {
		sshRaw, err = json.Marshal(sshConfig)
		if err != nil {
			return nil, nil, errors.New("catalog: encode ssh config")
		}
	}
	return tlsRaw, sshRaw, nil
}

func validateTLS(driver string, config *TLSConfig, secrets map[string]string) error {
	if config == nil || config.empty() {
		return nil
	}
	d, err := engine.New(driver)
	if err != nil {
		return validationError("driver", "Driver is not supported.")
	}
	capability, ok := d.(engine.TLSCapable)
	if !ok {
		return validationError("tls_config", "This driver does not support TLS configuration.")
	}
	spec := capability.TLSSpec()
	mode := engine.TLSMode(config.Mode)
	if !engine.ValidTLSMode(mode) || !slices.Contains(spec.Modes, mode) {
		return validationError("tls_config.mode", "Unsupported TLS verification mode.")
	}
	if config.ClientCertPEM == "" && secrets[string(credentials.SecretTLSClientKey)] != "" {
		return validationError("tls_config", "A client key requires a client certificate.")
	}
	return nil
}

func validateSSH(driver string, config *SSHConfig, secrets map[string]string) error {
	if config == nil || config.empty() || !config.Enabled {
		return nil
	}
	d, err := engine.New(driver)
	if err != nil {
		return validationError("driver", "Driver is not supported.")
	}
	capability, ok := d.(engine.SSHTunnelCapable)
	if !ok || !capability.SupportsSSHTunnel() {
		return validationError("ssh_config", "This driver does not support SSH tunneling.")
	}
	if strings.TrimSpace(config.Host) == "" {
		return validationError("ssh_config.host", "SSH host is required.")
	}
	if strings.TrimSpace(config.User) == "" {
		return validationError("ssh_config.user", "SSH user is required.")
	}
	if config.Port != 0 && (config.Port < 1 || config.Port > 65535) {
		return validationError("ssh_config.port", "SSH port must be between 1 and 65535.")
	}
	switch credentials.SSHAuthMethod(config.AuthMethod) {
	case credentials.SSHAuthPassword:
		if secrets[string(credentials.SecretSSHPassword)] == "" {
			return validationError("secrets.ssh_password", "SSH password is required for password authentication.")
		}
	case credentials.SSHAuthPrivateKey:
		privateKey := secrets[string(credentials.SecretSSHPrivateKey)]
		if privateKey == "" {
			return validationError("secrets.ssh_private_key", "SSH private key is required for key authentication.")
		}
		if privateKey == referenceSecretMarker {
			break
		}
		passphrase := secrets[string(credentials.SecretSSHPassphrase)]
		if passphrase == referenceSecretMarker {
			break
		}
		var parseErr error
		if passphrase != "" {
			_, parseErr = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		} else {
			_, parseErr = ssh.ParsePrivateKey([]byte(privateKey))
		}
		if parseErr != nil {
			return validationError("secrets.ssh_private_key", "SSH private key could not be parsed (check the passphrase).")
		}
	default:
		return validationError("ssh_config.auth_method", "SSH auth method must be password or private_key.")
	}
	if !config.InsecureSkipHostKey {
		switch {
		case config.KnownHostsEntry != "":
			if _, _, _, _, _, err := ssh.ParseKnownHosts([]byte(config.KnownHostsEntry)); err != nil {
				return validationError("ssh_config.known_hosts_entry", "known_hosts entry could not be parsed.")
			}
		case config.Fingerprint != "":
			if !strings.HasPrefix(config.Fingerprint, "SHA256:") {
				return validationError("ssh_config.fingerprint", "Host key fingerprint must be in SHA256:... form.")
			}
		default:
			return validationError("ssh_config", "Provide a known_hosts entry or SHA256 fingerprint, or explicitly disable host key verification.")
		}
	}
	return nil
}

func tlsToEngine(config *TLSConfig, secrets map[string]string) *engine.TLSConfig {
	if config == nil || config.empty() {
		return nil
	}
	return &engine.TLSConfig{Mode: engine.TLSMode(config.Mode), ServerName: config.ServerName, CAPEM: config.CAPEM, ClientCertPEM: config.ClientCertPEM, ClientKeyPEM: secrets[string(credentials.SecretTLSClientKey)]}
}

func sshToEngine(config *SSHConfig, secrets map[string]string) *credentials.SSHConfig {
	if config == nil || !config.Enabled {
		return nil
	}
	return &credentials.SSHConfig{Host: config.Host, Port: config.Port, User: config.User, AuthMethod: credentials.SSHAuthMethod(config.AuthMethod), Password: secrets[string(credentials.SecretSSHPassword)], PrivateKeyPEM: secrets[string(credentials.SecretSSHPrivateKey)], Passphrase: secrets[string(credentials.SecretSSHPassphrase)], KnownHostsEntry: config.KnownHostsEntry, Fingerprint: config.Fingerprint, InsecureSkipHostKey: config.InsecureSkipHostKey}
}

func driverSupportsSystemSchemas(driverName string) bool {
	set, ok := engine.Describe(driverName)
	return ok && set.Tree != nil && set.Tree.SystemObjects
}

func resolveShowAllDatabases(tree *metadata.Tree, defaultScope metadata.ScopePath, requested bool) bool {
	if tree == nil || tree.DatabaseKind() == "" {
		return false
	}
	if defaultScope.Name(tree.DatabaseKind()) == "" {
		return true
	}
	return requested
}
