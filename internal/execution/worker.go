package execution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/exports"
)

const maxRPCResponseBytes = 256 << 20

type transportFailureKind uint8

const (
	transportFailurePlain transportFailureKind = iota
	transportFailureOutcomeUnknown
	transportFailureTransactionLost
	transportFailureCursorLost
)

// WorkerRuntime implements SessionRuntime through one internal HTTP connector.
// It never retries operations, so an ambiguous write cannot execute twice.
type WorkerRuntime struct {
	directory   SessionDirectory
	authority   *GrantAuthority
	credentials ClientTransportCredentials
	client      *http.Client
	scopes      sync.Map // SessionHandle -> Scope
}

// NewWorkerRuntime returns a remote runtime routed by directory.
func NewWorkerRuntime(directory SessionDirectory, authority *GrantAuthority, credentials ClientTransportCredentials) (*WorkerRuntime, error) {
	if directory == nil || authority == nil {
		return nil, errors.New("worker runtime requires session directory and grant authority")
	}
	if credentials == nil {
		credentials = InsecureTransportCredentials{}
	}
	return &WorkerRuntime{
		directory: directory, authority: authority, credentials: credentials,
		client: credentials.HTTPClient(),
	}, nil
}

// Open opens a connector-owned target session.
func (r *WorkerRuntime) Open(ctx context.Context, request OpenRequest) (OpenResult, error) {
	grant, err := r.issue(request.Scope, request.Grant, rpcOpen)
	if err != nil {
		return OpenResult{}, err
	}
	request.Grant = grant
	var result OpenResult
	if err := r.call(ctx, "", rpcOpen, request, &result, transportFailureOutcomeUnknown); err != nil {
		return OpenResult{}, err
	}
	r.scopes.Store(result.Handle, request.Scope)
	return result, nil
}

// Query runs a row-producing operation on the connector.
func (r *WorkerRuntime) Query(ctx context.Context, request QueryRequest) (QueryResult, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcQuery); err != nil {
		return QueryResult{}, err
	}
	failure := transportFailurePlain
	if request.UseCursor {
		failure = transportFailureCursorLost
	} else if request.Explain != nil {
		failure = transportFailureOutcomeUnknown
	}
	var result QueryResult
	if err := r.call(ctx, request.Handle, rpcQuery, request, &result, failure); err != nil {
		return QueryResult{}, err
	}
	return result, nil
}

// Fetch retrieves the next remote cursor page without replaying on failure.
func (r *WorkerRuntime) Fetch(ctx context.Context, request FetchRequest) (FetchResult, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcFetch); err != nil {
		return FetchResult{}, err
	}
	var result FetchResult
	if err := r.call(ctx, request.Handle, rpcFetch, request, &result, transportFailureCursorLost); err != nil {
		return FetchResult{}, err
	}
	return result, nil
}

// Execute runs a remote statement without transport retries.
func (r *WorkerRuntime) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcExecute); err != nil {
		return ExecuteResult{}, err
	}
	var result ExecuteResult
	if err := r.call(ctx, request.Handle, rpcExecute, request, &result, transportFailureOutcomeUnknown); err != nil {
		return ExecuteResult{}, err
	}
	return result, nil
}

// TransactionStatus returns a remote transaction snapshot.
func (r *WorkerRuntime) TransactionStatus(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcTransactionStatus); err != nil {
		return TransactionStatus{}, err
	}
	var result TransactionStatus
	if err := r.call(ctx, request.Handle, rpcTransactionStatus, request, &result, transportFailurePlain); err != nil {
		return TransactionStatus{}, err
	}
	return result, nil
}

// SetTransactionMode changes the remote session transaction mode.
func (r *WorkerRuntime) SetTransactionMode(ctx context.Context, request SessionRequest, mode TransactionMode) (TransactionStatus, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcSetTransactionMode); err != nil {
		return TransactionStatus{}, err
	}
	var result TransactionStatus
	input := transactionModeRequest{SessionRequest: request, Mode: mode}
	if err := r.call(ctx, request.Handle, rpcSetTransactionMode, input, &result, transportFailurePlain); err != nil {
		return TransactionStatus{}, err
	}
	return result, nil
}

// Commit commits the remote transaction without replaying an ambiguous call.
func (r *WorkerRuntime) Commit(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcCommit); err != nil {
		return TransactionStatus{}, err
	}
	var result TransactionStatus
	if err := r.call(ctx, request.Handle, rpcCommit, request, &result, transportFailureTransactionLost); err != nil {
		return TransactionStatus{}, err
	}
	return result, nil
}

// Rollback rolls back the remote transaction without replaying an ambiguous call.
func (r *WorkerRuntime) Rollback(ctx context.Context, request SessionRequest) (TransactionStatus, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcRollback); err != nil {
		return TransactionStatus{}, err
	}
	var result TransactionStatus
	if err := r.call(ctx, request.Handle, rpcRollback, request, &result, transportFailureTransactionLost); err != nil {
		return TransactionStatus{}, err
	}
	return result, nil
}

// Cancel tears down a remote session after an interrupted operation.
func (r *WorkerRuntime) Cancel(ctx context.Context, request SessionRequest) error {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcCancel); err != nil {
		return err
	}
	return r.call(ctx, request.Handle, rpcCancel, request, nil, transportFailurePlain)
}

// Close releases a remote cursor or session.
func (r *WorkerRuntime) Close(ctx context.Context, request CloseRequest) error {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcClose); err != nil {
		return err
	}
	err := r.call(ctx, request.Handle, rpcClose, request, nil, transportFailurePlain)
	if err == nil && request.Cursor == "" {
		r.scopes.Delete(request.Handle)
	}
	return err
}

// Session returns safe metadata for a connector-owned session.
func (r *WorkerRuntime) Session(ctx context.Context, handle SessionHandle) (SessionInfo, bool, error) {
	// Session metadata is safe routing information. A signed empty-scope lookup
	// lets any API replica recover the exact scope before issuing an operation
	// grant, avoiding affinity to the API replica that opened the session.
	grant, err := r.issue(Scope{}, Grant{}, rpcSession)
	if err != nil {
		return SessionInfo{}, false, err
	}
	var result struct {
		Info  SessionInfo `json:"info"`
		Found bool        `json:"found"`
	}
	if err := r.call(ctx, handle, rpcSession, sessionLookupRequest{Handle: handle, Grant: grant}, &result, transportFailurePlain); err != nil {
		if errors.Is(err, ErrSessionLost) {
			r.scopes.Delete(handle)
			return SessionInfo{}, false, nil
		}
		return SessionInfo{}, false, err
	}
	if result.Found {
		r.scopes.Store(handle, result.Info.Scope)
	}
	return result.Info, result.Found, nil
}

// Sessions lists connector-owned sessions matching a scope.
func (r *WorkerRuntime) Sessions(ctx context.Context, scope Scope) ([]SessionInfo, error) {
	grant, err := r.issue(scope, Grant{}, rpcSessions)
	if err != nil {
		return nil, err
	}
	var result sessionsResponse
	if err := r.call(ctx, "", rpcSessions, sessionsRequest{Scope: scope, Grant: grant}, &result, transportFailurePlain); err != nil {
		return nil, err
	}
	for _, info := range result.Sessions {
		r.scopes.Store(info.Handle, info.Scope)
	}
	return result.Sessions, nil
}

// CloseMatching closes every connector-owned session matching a scope.
func (r *WorkerRuntime) CloseMatching(ctx context.Context, scope Scope) (int, error) {
	sessions, err := r.Sessions(ctx, scope)
	if err != nil {
		return 0, err
	}
	for _, info := range sessions {
		if err := r.Close(ctx, CloseRequest{Handle: info.Handle, Grant: Grant{Scope: info.Scope}}); err != nil {
			return 0, err
		}
	}
	return len(sessions), nil
}

// CountForConnection returns the remote session count, or zero when unavailable.
func (r *WorkerRuntime) CountForConnection(connectionID string) int {
	sessions, err := r.Sessions(context.Background(), Scope{ConnectionID: connectionID})
	if err != nil {
		return 0
	}
	return len(sessions)
}

// RemoveForConnection closes all remote sessions for a connection.
func (r *WorkerRuntime) RemoveForConnection(connectionID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{ConnectionID: connectionID})
	return removed
}

// RemoveForOrgAccount closes an account's remote sessions in an organization.
func (r *WorkerRuntime) RemoveForOrgAccount(orgID, accountID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{TenantID: orgID, AccountID: accountID})
	return removed
}

// RemoveForWorkspaceAccount closes an account's remote sessions in a workspace.
func (r *WorkerRuntime) RemoveForWorkspaceAccount(workspaceID, accountID string) int {
	removed, _ := r.CloseMatching(context.Background(), Scope{WorkspaceID: workspaceID, AccountID: accountID})
	return removed
}

// Capabilities reports optional operations implemented by the remote driver.
func (r *WorkerRuntime) Capabilities(ctx context.Context, request SessionRequest) (SessionCapabilities, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcCapabilities); err != nil {
		return SessionCapabilities{}, err
	}
	var result SessionCapabilities
	if err := r.call(ctx, request.Handle, rpcCapabilities, request, &result, transportFailurePlain); err != nil {
		return SessionCapabilities{}, err
	}
	return result, nil
}

// SchemaDirectory inspects a remote live schema directory.
func (r *WorkerRuntime) SchemaDirectory(ctx context.Context, request SchemaRequest) (*metadata.Directory, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcSchemaDirectory); err != nil {
		return nil, err
	}
	var result metadata.Directory
	if err := r.call(ctx, request.Handle, rpcSchemaDirectory, request, &result, transportFailurePlain); err != nil {
		return nil, err
	}
	return &result, nil
}

// SchemaObjects inspects remote live object metadata.
func (r *WorkerRuntime) SchemaObjects(ctx context.Context, request SchemaRequest) ([]metadata.Object, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcSchemaObjects); err != nil {
		return nil, err
	}
	var result []metadata.Object
	if err := r.call(ctx, request.Handle, rpcSchemaObjects, request, &result, transportFailurePlain); err != nil {
		return nil, err
	}
	return result, nil
}

// SchemaRelationships inspects relationships in a remote live scope.
func (r *WorkerRuntime) SchemaRelationships(ctx context.Context, request SchemaRequest) (*metadata.RelationshipGraph, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcSchemaRelationships); err != nil {
		return nil, err
	}
	var result metadata.RelationshipGraph
	if err := r.call(ctx, request.Handle, rpcSchemaRelationships, request, &result, transportFailurePlain); err != nil {
		return nil, err
	}
	return &result, nil
}

// SchemaDefinition fetches one remote live object definition.
func (r *WorkerRuntime) SchemaDefinition(ctx context.Context, request SchemaRequest) (*metadata.Descriptor, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcSchemaDefinition); err != nil {
		return nil, err
	}
	var result *metadata.Descriptor
	if err := r.call(ctx, request.Handle, rpcSchemaDefinition, request, &result, transportFailurePlain); err != nil {
		return nil, err
	}
	return result, nil
}

// Stream copies a connector-side export to writer and reports final trailers.
func (r *WorkerRuntime) Stream(ctx context.Context, request SessionRequest, writer io.Writer, opts exports.StreamOptions) (exports.StreamResult, error) {
	if err := r.signHandleRequest(ctx, request.Handle, &request.Grant, rpcStream); err != nil {
		return exports.StreamResult{}, err
	}
	endpoint, err := r.endpoint(ctx, request.Handle, runtimeStreamPath)
	if err != nil {
		return exports.StreamResult{}, err
	}
	payload, err := json.Marshal(streamRequest{SessionRequest: request, Options: streamOptions{
		Format: opts.Format, SQL: opts.SQL, MaxBytes: opts.MaxBytes, PageSize: opts.PageSize,
	}})
	if err != nil {
		return exports.StreamResult{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return exports.StreamResult{}, err
	}
	httpRequest.GetBody = nil
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(httpRequest)
	if err != nil {
		return exports.StreamResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return exports.StreamResult{}, decodeHTTPError(response)
	}
	if _, err := io.Copy(writer, response.Body); err != nil {
		return exports.StreamResult{}, err
	}
	result := exports.StreamResult{}
	result.Rows, _ = strconv.ParseInt(response.Trailer.Get("X-SQLWarden-Stream-Rows"), 10, 64)
	result.Bytes, _ = strconv.ParseInt(response.Trailer.Get("X-SQLWarden-Stream-Bytes"), 10, 64)
	if opts.OnProgress != nil && result.Rows > 0 {
		opts.OnProgress(result.Rows, result.Bytes)
	}
	if encoded := response.Trailer.Get("X-SQLWarden-Stream-Error"); encoded != "" {
		data, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
		if decodeErr != nil {
			return result, decodeErr
		}
		var remote rpcError
		if err := json.Unmarshal(data, &remote); err != nil {
			return result, err
		}
		return result, remote.err()
	}
	return result, nil
}

func (r *WorkerRuntime) signHandleRequest(ctx context.Context, handle SessionHandle, grant *Grant, permission string) error {
	signed, err := r.grantForHandle(ctx, handle, *grant, permission)
	if err != nil {
		return err
	}
	*grant = signed
	return nil
}

func (r *WorkerRuntime) grantForHandle(ctx context.Context, handle SessionHandle, supplied Grant, permission string) (Grant, error) {
	scope := supplied.Scope
	if scope == (Scope{}) {
		value, ok := r.scopes.Load(handle)
		if !ok {
			info, found, err := r.Session(ctx, handle)
			if err != nil {
				return Grant{}, err
			}
			if !found {
				return Grant{}, sessionLost(nil)
			}
			value = info.Scope
		}
		scope = value.(Scope)
	}
	return r.issue(scope, supplied, permission)
}

func (r *WorkerRuntime) issue(scope Scope, supplied Grant, permission string) (Grant, error) {
	permissions := []string{permission}
	for _, granted := range supplied.Permissions {
		if granted != permission {
			permissions = append(permissions, granted)
		}
	}
	return r.authority.Issue(scope, permissions, supplied.RevocationGeneration)
}

func (r *WorkerRuntime) call(ctx context.Context, handle SessionHandle, method string, input, output any, failure transportFailureKind) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	body, err := json.Marshal(rpcEnvelope{Method: method, Payload: payload})
	if err != nil {
		return err
	}
	endpoint, err := r.endpoint(ctx, handle, runtimeRPCPath)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	// Disabling GetBody makes the no-replay guarantee explicit even if a future
	// transport policy adds idempotency handling to the standard client.
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return classifyTransportFailure(failure, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return classifyRemoteFailure(failure, decodeHTTPError(response))
	}
	var result rpcResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxRPCResponseBytes))
	if err := decoder.Decode(&result); err != nil {
		return classifyTransportFailure(failure, err)
	}
	if result.Error != nil {
		return classifyRemoteFailure(failure, result.Error.err())
	}
	if output != nil && len(result.Payload) > 0 {
		if err := json.Unmarshal(result.Payload, output); err != nil {
			return classifyTransportFailure(failure, err)
		}
	}
	return nil
}

func classifyRemoteFailure(kind transportFailureKind, err error) error {
	if !errors.Is(err, ErrSessionLost) {
		return err
	}
	switch kind {
	case transportFailureTransactionLost:
		return transactionLost(err)
	case transportFailureCursorLost:
		return cursorLost(err)
	default:
		return err
	}
}

func (r *WorkerRuntime) endpoint(ctx context.Context, handle SessionHandle, path string) (string, error) {
	record, found, err := r.directory.Get(ctx, handle)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(record.RoutingAddress) == "" {
		return "", sessionLost(nil)
	}
	if record.ProtocolVersion != ProtocolVersion {
		return "", ErrProtocolMismatch
	}
	address := strings.TrimSpace(record.RoutingAddress)
	if parsed, parseErr := url.Parse(address); parseErr == nil && parsed.Scheme != "" {
		return strings.TrimRight(address, "/") + path, nil
	}
	return r.credentials.Scheme() + "://" + address + path, nil
}

func classifyTransportFailure(kind transportFailureKind, err error) error {
	switch kind {
	case transportFailureOutcomeUnknown:
		return outcomeUnknown(err)
	case transportFailureTransactionLost:
		return transactionLost(err)
	case transportFailureCursorLost:
		return cursorLost(err)
	default:
		return fmt.Errorf("execution transport: %w", err)
	}
}

func decodeHTTPError(response *http.Response) error {
	var result rpcResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, maxRPCResponseBytes)).Decode(&result); err == nil && result.Error != nil {
		return result.Error.err()
	}
	if response.StatusCode == http.StatusNotFound {
		return ErrProtocolMismatch
	}
	return fmt.Errorf("execution RPC status %d", response.StatusCode)
}

var _ SessionRuntime = (*WorkerRuntime)(nil)
