package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/pkg/result"
)

// ProtocolVersion is the execution contract version recorded in directory leases.
const ProtocolVersion uint32 = 1

// SSHAuthMethod identifies a supported tunnel authentication mechanism.
type SSHAuthMethod string

const (
	SSHAuthPassword   SSHAuthMethod = "password"
	SSHAuthPrivateKey SSHAuthMethod = "private_key"
)

// SSHTunnel exposes only the transport operations needed by target opening.
type SSHTunnel struct{ tunnel *connection.Tunnel }

// OpenSSHTunnel establishes a tunnel from a wire-safe runtime configuration.
func OpenSSHTunnel(ctx context.Context, config SSHConfig) (*SSHTunnel, error) {
	tunnel, err := connection.OpenTunnel(ctx, toConnectionSSH(config))
	if err != nil {
		return nil, err
	}
	return &SSHTunnel{tunnel: tunnel}, nil
}

// DialContext opens a connection through the tunnel.
func (t *SSHTunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return t.tunnel.DialContext(ctx, network, address)
}

// Healthy reports whether the tunnel transport is still usable.
func (t *SSHTunnel) Healthy() bool { return t.tunnel.Healthy() }

// Close releases the tunnel transport.
func (t *SSHTunnel) Close() error { return t.tunnel.Close() }

// LocalRuntime implements the execution boundary in-process by adapting the
// existing session and cursor managers. It adds no network hop.
type LocalRuntime struct {
	sessions  *connection.Manager
	cursors   *connection.QueryCursorManager
	directory SessionDirectory
	leaseTTL  time.Duration
	now       func() time.Time
}

// NewLocalRuntime adapts the existing in-process session and cursor managers
// to the process-independent execution contract.
func NewLocalRuntime(sessions *connection.Manager, cursors *connection.QueryCursorManager, directory SessionDirectory, leaseTTL time.Duration) *LocalRuntime {
	if directory == nil {
		directory = NewMemorySessionDirectory()
	}
	return &LocalRuntime{sessions: sessions, cursors: cursors, directory: directory, leaseTTL: leaseTTL, now: time.Now}
}

// Open creates or reuses a target session and publishes its directory lease.
func (r *LocalRuntime) Open(ctx context.Context, request OpenRequest) (OpenResult, error) {
	var tunnel *connection.Tunnel
	open := func() (engine.Driver, func(), error) {
		var err error
		if request.Target.SSH != nil {
			tunnel, err = connection.OpenTunnel(ctx, toConnectionSSH(*request.Target.SSH))
			if err != nil {
				return nil, nil, fmt.Errorf("ssh tunnel: %w", err)
			}
		}
		teardown := func() {}
		if tunnel != nil {
			teardown = func() { _ = tunnel.Close() }
		}

		driver, err := engine.New(request.Target.Driver)
		if err != nil {
			teardown()
			return nil, nil, err
		}
		config := engine.ConnectionConfig{
			DSN:            request.Target.DSN,
			Driver:         request.Target.Driver,
			DefaultScope:   request.Target.DefaultScope,
			MaxResultRows:  request.Target.Limits.MaxRows,
			MaxResultBytes: request.Target.Limits.MaxBytes,
			TLS:            request.Target.TLS,
		}
		if tunnel != nil {
			config.SSHDialer = tunnel.DialContext
		}
		if err := driver.Connect(ctx, config); err != nil {
			teardown()
			return nil, nil, err
		}
		return driver, teardown, nil
	}
	metadata := connection.SessionMetadata{OrgID: request.Scope.TenantID, WorkspaceID: request.Scope.WorkspaceID}
	var session *connection.Session
	created := true
	var err error
	if request.Ephemeral {
		session, err = r.sessions.CreateWithMetadata(request.Scope.AccountID, request.Scope.ConnectionID, metadata, open)
	} else {
		session, created, err = r.sessions.GetOrCreateWithMetadata(request.Scope.AccountID, request.Scope.ConnectionID, metadata, open)
	}
	if err != nil {
		return OpenResult{}, err
	}
	if created && tunnel != nil {
		t := tunnel
		session.SetTunnelHealth(func() *bool { healthy := t.Healthy(); return &healthy })
	}

	handle := SessionHandle(session.ID)
	now := r.now()
	record := DirectoryRecord{
		Handle: handle, Scope: request.Scope, OwnerRuntimeID: "local", RoutingAddress: "in-process", CreatedAt: now,
		LastSeenAt: now, LeaseExpiresAt: now.Add(r.leaseTTL), ProtocolVersion: ProtocolVersion,
		RevocationGeneration: request.Grant.RevocationGeneration,
	}
	if !created {
		if current, ok, getErr := r.directory.Get(ctx, handle); getErr == nil && ok {
			record.CreatedAt = current.CreatedAt
		}
	}
	if err := r.directory.Put(ctx, record); err != nil {
		if created {
			r.sessions.Remove(session.ID)
		}
		return OpenResult{}, err
	}
	return OpenResult{Handle: handle, Reused: !created}, nil
}

// Query executes a row-producing operation and optionally creates a cursor.
func (r *LocalRuntime) Query(ctx context.Context, request QueryRequest) (QueryResult, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return QueryResult{}, err
	}
	var rs *result.ResultSet
	resultValue := QueryResult{}
	if request.Explain != nil {
		rs, err = session.ExecuteExplainPlan(ctx, *request.Explain, scanOptions(request.Limits, 0))
	} else if request.UseCursor {
		var opened *connection.QueryCursorHandle
		opened, err = session.StartQueryCursor(context.WithoutCancel(ctx), request.SQL, request.Args...)
		if err == nil {
			record := r.cursors.Create(connection.QueryCursorCreateParams{ParentSession: session, Cursor: opened})
			var state cursor.QueryCursorState
			var fetchErr error
			rs, state, fetchErr = opened.Fetch(ctx, scanOptions(request.Limits, request.PageSize))
			if fetchErr != nil {
				r.cursors.Remove(record.ID)
				err = fetchErr
			} else {
				resultValue.Cursor = CursorHandle(record.ID)
				resultValue.Exhausted = state.Exhausted
				if state.Exhausted {
					record.MarkExhausted()
					r.cursors.Remove(record.ID)
				}
			}
		}
	} else {
		rs, err = session.QueryWithOptions(ctx, request.SQL, scanOptions(request.Limits, 0), request.Args...)
	}
	if err != nil {
		if request.Explain != nil && isContextError(err) {
			return QueryResult{}, outcomeUnknown(err)
		}
		return QueryResult{}, normalizeError(err)
	}
	resultValue.Result = rs
	resultValue.Transaction = transactionStatus(session.TransactionStatus())
	return resultValue, nil
}

// Fetch returns the next page from a cursor owned by the selected session.
func (r *LocalRuntime) Fetch(ctx context.Context, request FetchRequest) (FetchResult, error) {
	if _, err := r.session(ctx, request.Handle); err != nil {
		return FetchResult{}, err
	}
	record, ok := r.cursors.Get(string(request.Cursor))
	if !ok || record.ParentSessionID != string(request.Handle) || record.ParentSession == nil {
		return FetchResult{}, cursorLost(nil)
	}
	if !record.Touch() {
		r.cursors.Remove(record.ID)
		return FetchResult{}, cursorLost(nil)
	}
	rs, state, err := record.Cursor.Fetch(ctx, scanOptions(request.Limits, request.PageSize))
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			r.cursors.Remove(record.ID)
		}
		return FetchResult{}, normalizeError(err)
	}
	if state.Exhausted {
		record.MarkExhausted()
		r.cursors.Remove(record.ID)
	}
	return FetchResult{Result: rs, Exhausted: state.Exhausted}, nil
}

// Execute runs a statement, explain plan, or structured DDL operation.
func (r *LocalRuntime) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return ExecuteResult{}, err
	}
	var rs *result.ResultSet
	switch {
	case request.DDL != nil:
		err = session.ApplyDDL(ctx, *request.DDL)
	case request.Explain != nil:
		rs, err = session.ExecuteExplainPlan(ctx, *request.Explain, scanOptions(request.Limits, 0))
	default:
		rs, err = session.ExecuteWithOptions(ctx, request.SQL, scanOptions(request.Limits, 0), request.Args...)
	}
	if err != nil {
		if isContextError(err) {
			return ExecuteResult{}, outcomeUnknown(err)
		}
		return ExecuteResult{}, normalizeError(err)
	}
	return ExecuteResult{Result: rs, Transaction: transactionStatus(session.TransactionStatus())}, nil
}

// TransactionStatus returns the selected session's transaction snapshot.
func (r *LocalRuntime) TransactionStatus(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return TransactionStatus{}, err
	}
	return transactionStatus(session.TransactionStatus()), nil
}

// SetTransactionMode changes auto-commit behavior for the selected session.
func (r *LocalRuntime) SetTransactionMode(ctx context.Context, request SessionRequest, mode TransactionMode) (TransactionStatus, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return TransactionStatus{}, err
	}
	if err := session.SetTransactionMode(ctx, connection.TxMode(mode)); err != nil {
		return TransactionStatus{}, normalizeError(err)
	}
	return transactionStatus(session.TransactionStatus()), nil
}

// Commit commits the selected session's open transaction.
func (r *LocalRuntime) Commit(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return TransactionStatus{}, transactionLost(err)
	}
	if err := session.CommitTransaction(ctx); err != nil {
		err = normalizeTransactionError(err)
		if errors.Is(err, ErrNoOpenTransaction) {
			return TransactionStatus{}, err
		}
		return TransactionStatus{}, transactionLost(err)
	}
	return transactionStatus(session.TransactionStatus()), nil
}

// Rollback rolls back the selected session's open transaction.
func (r *LocalRuntime) Rollback(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return TransactionStatus{}, transactionLost(err)
	}
	if err := session.RollbackTransaction(ctx); err != nil {
		err = normalizeTransactionError(err)
		if errors.Is(err, ErrNoOpenTransaction) {
			return TransactionStatus{}, err
		}
		return TransactionStatus{}, transactionLost(err)
	}
	return transactionStatus(session.TransactionStatus()), nil
}

// Cancel tears down a session after an interrupted operation.
func (r *LocalRuntime) Cancel(ctx context.Context, request SessionRequest) error {
	if _, err := r.session(ctx, request.Handle); err != nil {
		return err
	}
	r.sessions.Remove(string(request.Handle))
	return r.directory.Delete(ctx, request.Handle)
}

// Close releases a cursor when supplied, or otherwise the entire session.
func (r *LocalRuntime) Close(ctx context.Context, request CloseRequest) error {
	if request.Cursor != "" {
		record, ok := r.cursors.Get(string(request.Cursor))
		if !ok || record.ParentSessionID != string(request.Handle) {
			return nil
		}
		r.cursors.Remove(record.ID)
		return nil
	}
	r.sessions.Remove(string(request.Handle))
	return r.directory.Delete(ctx, request.Handle)
}

// Session returns safe ownership metadata for one live session.
func (r *LocalRuntime) Session(ctx context.Context, handle SessionHandle) (SessionInfo, bool, error) {
	session, ok := r.sessions.Get(string(handle))
	if !ok {
		_ = r.directory.Delete(ctx, handle)
		return SessionInfo{}, false, nil
	}
	if err := r.renew(ctx, handle); err != nil {
		return SessionInfo{}, false, err
	}
	return sessionInfo(session), true, nil
}

// Sessions lists live sessions matching every non-empty scope field.
func (r *LocalRuntime) Sessions(_ context.Context, scope Scope) ([]SessionInfo, error) {
	var refs []connection.SessionRef
	if scope.AccountID != "" {
		refs = r.sessions.AllForAccount(scope.AccountID)
	} else if scope.WorkspaceID != "" {
		refs = r.sessions.AllForWorkspace(scope.WorkspaceID)
	} else {
		refs = r.sessions.All()
	}
	infos := make([]SessionInfo, 0, len(refs))
	for _, ref := range refs {
		info := SessionInfo{Handle: SessionHandle(ref.SessionID), Scope: Scope{
			TenantID: ref.OrgID, AccountID: ref.AccountID, WorkspaceID: ref.WorkspaceID, ConnectionID: ref.ConnectionID,
		}, TunnelHealthy: ref.TunnelHealthy}
		if matchesScope(info.Scope, scope) {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

// CloseMatching closes all live sessions matching a scope.
func (r *LocalRuntime) CloseMatching(ctx context.Context, scope Scope) (int, error) {
	infos, err := r.Sessions(ctx, scope)
	if err != nil {
		return 0, err
	}
	for _, info := range infos {
		if err := r.Close(ctx, CloseRequest{Handle: info.Handle}); err != nil {
			return 0, err
		}
	}
	return len(infos), nil
}

// CountForConnection reports the number of live sessions for a connection.
func (r *LocalRuntime) CountForConnection(connectionID string) int {
	return r.sessions.CountForConnection(connectionID)
}

// RemoveForConnection closes every live session for a connection.
func (r *LocalRuntime) RemoveForConnection(connectionID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{ConnectionID: connectionID})
	return removed
}

// RemoveForOrgAccount closes an account's live sessions in an organization.
func (r *LocalRuntime) RemoveForOrgAccount(orgID, accountID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{TenantID: orgID, AccountID: accountID})
	return removed
}

// RemoveForWorkspaceAccount closes an account's live sessions in a workspace.
func (r *LocalRuntime) RemoveForWorkspaceAccount(workspaceID, accountID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{WorkspaceID: workspaceID, AccountID: accountID})
	return removed
}

// Capabilities reports optional operations implemented by a live driver.
func (r *LocalRuntime) Capabilities(ctx context.Context, request SessionRequest) (SessionCapabilities, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return SessionCapabilities{}, err
	}
	capabilities := SessionCapabilities{}
	if inspector, ok := session.Conn.(metadata.SchemaInspector); ok {
		spec := inspector.SchemaSpec()
		capabilities.Schema = &spec
	}
	if executor, ok := session.Conn.(ddl.Executor); ok {
		spec := executor.DDLSpec()
		capabilities.DDL = &spec
	}
	_, capabilities.Relationships = session.Conn.(metadata.RelationshipInspector)
	_, capabilities.Definitions = session.Conn.(metadata.DefinitionInspector)
	return capabilities, nil
}

// SchemaDirectory inspects the live schema directory at an optional root.
func (r *LocalRuntime) SchemaDirectory(ctx context.Context, request SchemaRequest) (*metadata.Directory, error) {
	inspector, _, err := r.schemaInspector(ctx, request.Handle)
	if err != nil {
		return nil, err
	}
	return inspector.InspectDirectory(ctx, metadata.DirectoryOptions{Root: request.Scope})
}

// SchemaObjects inspects live metadata for the requested object references.
func (r *LocalRuntime) SchemaObjects(ctx context.Context, request SchemaRequest) ([]metadata.Object, error) {
	inspector, _, err := r.schemaInspector(ctx, request.Handle)
	if err != nil {
		return nil, err
	}
	return inspector.InspectObjects(ctx, request.Refs)
}

// SchemaRelationships inspects relationships in one live schema scope.
func (r *LocalRuntime) SchemaRelationships(ctx context.Context, request SchemaRequest) (*metadata.RelationshipGraph, error) {
	_, driver, err := r.schemaInspector(ctx, request.Handle)
	if err != nil {
		return nil, err
	}
	inspector, ok := driver.(metadata.RelationshipInspector)
	if !ok {
		return nil, ErrRelationshipsUnsupported
	}
	return inspector.InspectRelationshipsInScope(ctx, request.Scope)
}

// SchemaDefinition fetches one live object definition.
func (r *LocalRuntime) SchemaDefinition(ctx context.Context, request SchemaRequest) (*metadata.Descriptor, error) {
	_, driver, err := r.schemaInspector(ctx, request.Handle)
	if err != nil {
		return nil, err
	}
	inspector, ok := driver.(metadata.DefinitionInspector)
	if !ok || request.Ref == nil {
		return nil, ErrSchemaUnsupported
	}
	return inspector.InspectDefinition(ctx, *request.Ref)
}

// Stream exports query results directly from the selected session.
func (r *LocalRuntime) Stream(ctx context.Context, request SessionRequest, writer io.Writer, opts exports.StreamOptions) (exports.StreamResult, error) {
	session, err := r.session(ctx, request.Handle)
	if err != nil {
		return exports.StreamResult{}, err
	}
	return exports.NewService().Stream(ctx, session.Conn, writer, opts)
}

func (r *LocalRuntime) session(ctx context.Context, handle SessionHandle) (*connection.Session, error) {
	session, ok := r.sessions.Get(string(handle))
	if !ok {
		_ = r.directory.Delete(ctx, handle)
		return nil, sessionLost(nil)
	}
	if err := r.renew(ctx, handle); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *LocalRuntime) renew(ctx context.Context, handle SessionHandle) error {
	return r.directory.Renew(ctx, handle, r.now().Add(r.leaseTTL))
}

func (r *LocalRuntime) schemaInspector(ctx context.Context, handle SessionHandle) (metadata.SchemaInspector, engine.Driver, error) {
	session, err := r.session(ctx, handle)
	if err != nil {
		return nil, nil, err
	}
	inspector, ok := session.Conn.(metadata.SchemaInspector)
	if !ok {
		return nil, nil, ErrSchemaUnsupported
	}
	return inspector, session.Conn, nil
}

func sessionInfo(session *connection.Session) SessionInfo {
	return SessionInfo{Handle: SessionHandle(session.ID), Scope: Scope{
		TenantID: session.OrgID, AccountID: session.AccountID, WorkspaceID: session.WorkspaceID, ConnectionID: session.ConnectionID,
	}}
}

func matchesScope(actual, requested Scope) bool {
	return (requested.TenantID == "" || actual.TenantID == requested.TenantID) &&
		(requested.AccountID == "" || actual.AccountID == requested.AccountID) &&
		(requested.WorkspaceID == "" || actual.WorkspaceID == requested.WorkspaceID) &&
		(requested.ConnectionID == "" || actual.ConnectionID == requested.ConnectionID)
}

func transactionStatus(status connection.TransactionStatus) TransactionStatus {
	return TransactionStatus{
		Mode: TransactionMode(status.Mode), Open: status.Open,
		PendingStatements: status.PendingStatements, Statements: append([]string(nil), status.Statements...),
	}
}

func sessionLost(err error) error {
	if err == nil {
		err = ErrSessionLost
	}
	return &Failure{Code: FailureSessionLost, Retryable: true, Err: err}
}

func cursorLost(err error) error {
	if err == nil {
		err = ErrCursorLost
	}
	return &Failure{Code: FailureCursorLost, Retryable: true, Err: err}
}

func transactionLost(err error) error {
	if err == nil {
		err = ErrTransactionLost
	} else {
		err = errors.Join(ErrTransactionLost, err)
	}
	return &Failure{Code: FailureTransactionLost, Err: err}
}

func outcomeUnknown(err error) error {
	if err == nil {
		err = ErrOutcomeUnknown
	} else {
		err = errors.Join(ErrOutcomeUnknown, err)
	}
	return &Failure{Code: FailureExecutionOutcomeUnknown, Err: err}
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func normalizeError(err error) error {
	switch {
	case errors.Is(err, connection.ErrQueryCursorsUnsupported):
		return ErrQueryCursorUnsupported
	case errors.Is(err, cursor.ErrCursorClosed):
		return cursorLost(err)
	case errors.Is(err, connection.ErrTransactionOpen):
		return ErrTransactionOpen
	default:
		return err
	}
}

func normalizeTransactionError(err error) error {
	if errors.Is(err, connection.ErrTransactionOpen) {
		return ErrTransactionOpen
	}
	return err
}

func toConnectionSSH(config SSHConfig) connection.SSHConfig {
	return connection.SSHConfig{
		Host: config.Host, Port: config.Port, User: config.User,
		AuthMethod: connection.SSHAuthMethod(config.AuthMethod), Password: config.Password,
		PrivateKeyPEM: config.PrivateKeyPEM, Passphrase: config.Passphrase,
		KnownHostsEntry: config.KnownHostsEntry, Fingerprint: config.Fingerprint,
		InsecureSkipHostKey: config.InsecureSkipHostKey,
	}
}

// ParseNumericScope converts database identifiers to the transport-neutral
// string representation used by the runtime contract.
func ParseNumericScope(accountID, tenantID, workspaceID, connectionID int64) Scope {
	return Scope{
		AccountID: strconv.FormatInt(accountID, 10), TenantID: strconv.FormatInt(tenantID, 10),
		WorkspaceID: strconv.FormatInt(workspaceID, 10), ConnectionID: strconv.FormatInt(connectionID, 10),
	}
}

var _ SessionRuntime = (*LocalRuntime)(nil)
