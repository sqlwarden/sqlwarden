package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

const connectTimeout = 10 * time.Second

// LocalConfig wires a LocalRuntime to its collaborators.
type LocalConfig struct {
	Credentials credentials.Resolver
	Policy      TargetPolicy
	IdleTimeout time.Duration
	Logger      *slog.Logger
	// Manager and Cursors are an embedding seam: when set, the runtime operates
	// on sessions and cursors owned by the caller (for example a host that
	// registers sessions over its own drivers, or tests seeding fake drivers).
	// The caller keeps responsibility for closing them; production wiring
	// leaves them nil so the runtime owns its session state.
	Manager *connection.Manager
	Cursors *connection.QueryCursorManager
}

// LocalRuntime is the in-process execution runtime over connection.Manager.
type LocalRuntime struct {
	creds   credentials.Resolver
	policy  TargetPolicy
	manager *connection.Manager
	cursors *connection.QueryCursorManager
	logger  *slog.Logger
}

var (
	_ Queries      = (*LocalRuntime)(nil)
	_ Transactions = (*LocalRuntime)(nil)
	_ Sessions     = (*LocalRuntime)(nil)
	_ Revoker      = (*LocalRuntime)(nil)
	_ Prober       = (*LocalRuntime)(nil)
	_ Metadata     = (*LocalRuntime)(nil)
	_ Capabilities = (*LocalRuntime)(nil)
	_ Runtime      = (*LocalRuntime)(nil)
)

func NewLocal(cfg LocalConfig) *LocalRuntime {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	manager := cfg.Manager
	if manager == nil {
		manager = connection.New(cfg.IdleTimeout)
	}
	cursors := cfg.Cursors
	if cursors == nil {
		cursors = connection.NewQueryCursorManager(cursorIdleTimeout)
	}
	return &LocalRuntime{
		creds:   cfg.Credentials,
		policy:  cfg.Policy,
		manager: manager,
		cursors: cursors,
		logger:  logger,
	}
}

// OnConnectionEmpty registers a hook invoked, without internal locks held,
// after the last live session of a connection is removed.
func (r *LocalRuntime) OnConnectionEmpty(hook func(connectionID string)) {
	r.manager.SetOnConnectionEmpty(hook)
}

// Shutdown closes every session and stops background reaping. It is named
// Shutdown because Sessions.Close already takes a session.
func (r *LocalRuntime) Shutdown() {
	r.cursors.Close()
	r.manager.Close()
}

var (
	errInvalidScope      = errors.New("execution: scope requires org, workspace, account, and connection")
	errInvalidProbeScope = errors.New("execution: probe scope requires org, workspace, and account")
)

// TunnelError reports that the SSH tunnel to the target could not be opened,
// as distinct from the target itself refusing the connection.
type TunnelError struct {
	Err error
}

func (e *TunnelError) Error() string { return "ssh tunnel: " + e.Err.Error() }

func (e *TunnelError) Unwrap() error { return e.Err }

func sessionNotFound() error {
	return &Failure{Code: FailureSessionNotFound, Err: ErrSessionNotFound}
}

func isSessionNotFound(err error) bool { return errors.Is(err, ErrSessionNotFound) }

// session resolves id and verifies it belongs to scope. Unknown ids and any
// scope mismatch yield the identical error so callers cannot probe for the
// existence of other principals' sessions.
func (r *LocalRuntime) session(scope Scope, id SessionID) (*connection.Session, error) {
	sess, ok := r.manager.Get(string(id))
	if !ok || !ownedBy(sess, scope) {
		return nil, sessionNotFound()
	}
	return sess, nil
}

// ownedBy is the single scope-ownership predicate for sessions and for the
// cursors that hang off them.
func ownedBy(sess *connection.Session, scope Scope) bool {
	return sess != nil &&
		sess.AccountID == scope.AccountID &&
		sess.ConnectionID == scope.ConnectionID &&
		sess.OrgID == scope.OrgID &&
		sess.WorkspaceID == scope.WorkspaceID
}

func infoOf(sess *connection.Session, reused bool) SessionInfo {
	return SessionInfo{
		ID: SessionID(sess.ID),
		Scope: Scope{
			OrgID:        sess.OrgID,
			WorkspaceID:  sess.WorkspaceID,
			AccountID:    sess.AccountID,
			ConnectionID: sess.ConnectionID,
		},
		Driver:        sess.Driver,
		TunnelHealthy: sess.TunnelHealthy(),
		Reused:        reused,
	}
}

func (r *LocalRuntime) Open(ctx context.Context, req OpenRequest) (SessionInfo, error) {
	scope := req.Scope
	if !scope.Valid() {
		return SessionInfo{}, errInvalidScope
	}
	// Credentials and policy are re-evaluated even when a pooled session will
	// be reused, so target-policy changes apply to every open request.
	creds, err := r.creds.Resolve(ctx, credentials.ConnectionRef{
		OrgID:        scope.OrgID,
		WorkspaceID:  scope.WorkspaceID,
		ConnectionID: scope.ConnectionID,
	})
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			return SessionInfo{}, fmt.Errorf("execution: connection not found: %w", err)
		}
		return SessionInfo{}, fmt.Errorf("execution: resolve credentials: %w", err)
	}
	if err := r.policy.Check(ctx, creds.Driver, creds.DSN); err != nil {
		r.logger.WarnContext(ctx, "execution target denied by policy",
			slog.String("connection_id", scope.ConnectionID), slog.String("driver", creds.Driver))
		return SessionInfo{}, err
	}

	var tunnel *connection.Tunnel
	open := func() (engine.Driver, func(), error) {
		d, t, teardown, err := r.connect(ctx, creds, req.Limits)
		tunnel = t
		return d, teardown, err
	}
	meta := connection.SessionMetadata{OrgID: scope.OrgID, WorkspaceID: scope.WorkspaceID, Driver: creds.Driver}

	var (
		sess    *connection.Session
		created bool
	)
	if req.Ephemeral {
		sess, err = r.manager.CreatePrivate(scope.AccountID, scope.ConnectionID, meta, open)
		created = true
	} else {
		sess, created, err = r.manager.GetOrCreateWithMetadata(scope.AccountID, scope.ConnectionID, meta, open)
	}
	if err != nil {
		return SessionInfo{}, err
	}

	if created && tunnel != nil {
		t := tunnel
		sess.SetTunnelHealth(func() *bool { h := t.Healthy(); return &h })
	}
	r.logger.InfoContext(ctx, "database session opened",
		slog.String("connection_id", scope.ConnectionID),
		slog.String("session_id", sess.ID),
		slog.Bool("reused", !created),
		slog.Bool("ephemeral", req.Ephemeral))
	return infoOf(sess, !created), nil
}

// connect builds a connected driver, opening an SSH tunnel first when the
// credentials require one. The returned teardown releases the tunnel and is
// safe to call exactly once after the driver is closed.
func (r *LocalRuntime) connect(ctx context.Context, creds credentials.Credentials, limits Limits) (engine.Driver, *connection.Tunnel, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	var tunnel *connection.Tunnel
	teardown := func() {}
	if creds.SSH != nil {
		t, err := connection.OpenTunnel(ctx, *creds.SSH)
		if err != nil {
			return nil, nil, nil, &TunnelError{Err: err}
		}
		tunnel = t
		teardown = func() { _ = t.Close() }
	}

	d, err := engine.New(creds.Driver)
	if err != nil {
		teardown()
		return nil, nil, nil, &ConnectError{Err: err}
	}
	cc := engine.ConnectionConfig{
		DSN:            creds.DSN,
		Driver:         creds.Driver,
		DefaultScope:   metadata.ScopePath(creds.DefaultScope),
		TLS:            creds.TLS,
		MaxResultRows:  limits.MaxRows,
		MaxResultBytes: limits.MaxBytes,
	}
	if tunnel != nil {
		cc.SSHDialer = tunnel.DialContext
	}
	if err := d.Connect(ctx, cc); err != nil {
		teardown()
		return nil, nil, nil, &ConnectError{Err: err}
	}
	return d, tunnel, teardown, nil
}

func (r *LocalRuntime) Get(ctx context.Context, scope Scope, id SessionID) (SessionInfo, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return SessionInfo{}, err
	}
	return infoOf(sess, true), nil
}

// List applies the SessionFilter rule: both fields narrow to one account's
// sessions in one workspace; AccountID alone spans workspaces; WorkspaceID
// alone lists every account's sessions in the workspace, which callers must
// authorize themselves.
func (r *LocalRuntime) List(ctx context.Context, filter SessionFilter) ([]SessionInfo, error) {
	var refs []connection.SessionRef
	switch {
	case filter.AccountID == "" && filter.WorkspaceID == "":
		return nil, errors.New("execution: session filter requires an account or workspace")
	case filter.WorkspaceID == "":
		refs = r.manager.AllForAccount(filter.AccountID)
	default:
		refs = r.manager.AllForWorkspace(filter.WorkspaceID)
	}
	out := make([]SessionInfo, 0, len(refs))
	for _, ref := range refs {
		if filter.AccountID != "" && ref.AccountID != filter.AccountID {
			continue
		}
		out = append(out, SessionInfo{
			ID: SessionID(ref.SessionID),
			Scope: Scope{
				OrgID:        ref.OrgID,
				WorkspaceID:  ref.WorkspaceID,
				AccountID:    ref.AccountID,
				ConnectionID: ref.ConnectionID,
			},
			Driver:        ref.Driver,
			TunnelHealthy: ref.TunnelHealthy,
			Reused:        true,
		})
	}
	return out, nil
}

// Close ends a session. Closing an unknown or foreign session is a no-op so
// repeated disconnects stay idempotent without revealing which case applied.
func (r *LocalRuntime) Close(ctx context.Context, scope Scope, id SessionID) error {
	sess, err := r.session(scope, id)
	if err != nil {
		if isSessionNotFound(err) {
			return nil
		}
		return err
	}
	r.manager.Remove(sess.ID)
	return nil
}

func (r *LocalRuntime) CountForConnection(ctx context.Context, connID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.manager.CountForConnection(connID), nil
}

func (r *LocalRuntime) RevokeConnection(ctx context.Context, connID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.manager.RemoveForConnection(connID), nil
}

func (r *LocalRuntime) RevokeWorkspaceAccount(ctx context.Context, wsID, accountID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.manager.RemoveForWorkspaceAccount(wsID, accountID), nil
}

func (r *LocalRuntime) RevokeOrgAccount(ctx context.Context, orgID, accountID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.manager.RemoveForOrgAccount(orgID, accountID), nil
}

// Probe connects and pings before handing fn the inspector. It accepts a scope
// without ConnectionID because it also vets targets that have not been saved
// yet.
func (r *LocalRuntime) Probe(ctx context.Context, scope Scope, creds credentials.Credentials, limits Limits, fn func(metadata.SchemaInspector) error) error {
	if scope.OrgID == "" || scope.WorkspaceID == "" || scope.AccountID == "" {
		return errInvalidProbeScope
	}
	if err := r.policy.Check(ctx, creds.Driver, creds.DSN); err != nil {
		return err
	}
	d, _, teardown, err := r.connect(ctx, creds, limits)
	if err != nil {
		return err
	}
	defer teardown()
	defer func() { _ = d.Close() }()
	if err := d.Ping(ctx); err != nil {
		return &ConnectError{Err: err}
	}

	inspector, ok := d.(metadata.SchemaInspector)
	if !ok {
		return ErrSchemaUnsupported
	}
	return fn(inspector)
}
