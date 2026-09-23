package execution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/observability"
)

const maxRPCBodyBytes = 16 << 20

const (
	rpcOpen                = "open"
	rpcQuery               = "query"
	rpcFetch               = "fetch"
	rpcExecute             = "execute"
	rpcTransactionStatus   = "transaction_status"
	rpcSetTransactionMode  = "set_transaction_mode"
	rpcCommit              = "commit"
	rpcRollback            = "rollback"
	rpcCancel              = "cancel"
	rpcClose               = "close"
	rpcSession             = "session"
	rpcSessions            = "sessions"
	rpcCapabilities        = "capabilities"
	rpcSchemaDirectory     = "schema_directory"
	rpcSchemaObjects       = "schema_objects"
	rpcSchemaRelationships = "schema_relationships"
	rpcSchemaDefinition    = "schema_definition"
	rpcStream              = "stream"
)

type transactionModeRequest struct {
	SessionRequest
	Mode TransactionMode `json:"mode"`
}

type sessionLookupRequest struct {
	Handle SessionHandle `json:"session_handle"`
	Grant  Grant         `json:"grant"`
}

type sessionsRequest struct {
	Scope Scope `json:"scope"`
	Grant Grant `json:"grant"`
}

type sessionsResponse struct {
	Sessions []SessionInfo `json:"sessions"`
}

type streamRequest struct {
	SessionRequest
	Options streamOptions `json:"options"`
}

type streamOptions struct {
	Format   string `json:"format"`
	SQL      string `json:"sql"`
	MaxBytes int64  `json:"max_bytes"`
	PageSize int    `json:"page_size"`
}

// RuntimeServer exposes a SessionRuntime over an authenticated internal HTTP
// protocol. The containing process kind owns its listener and credentials.
type RuntimeServer struct {
	runtime   SessionRuntime
	authority *GrantAuthority
	handler   http.Handler
	logger    *slog.Logger
}

// NewRuntimeServer returns an internal execution RPC handler. Each request
// produces one outcome log; supply WithLogger to record them.
func NewRuntimeServer(runtime SessionRuntime, authority *GrantAuthority, opts ...Option) (*RuntimeServer, error) {
	if runtime == nil || authority == nil {
		return nil, errors.New("execution runtime server requires runtime and grant authority")
	}
	options := applyOptions(opts)
	server := &RuntimeServer{runtime: runtime, authority: authority, logger: options.logger}
	mux := http.NewServeMux()
	mux.HandleFunc(runtimeRPCPath, server.serveCall)
	mux.HandleFunc(runtimeStreamPath, server.serveStream)
	server.handler = mux
	return server, nil
}

// Handler returns the internal RPC HTTP handler.
func (s *RuntimeServer) Handler() http.Handler { return s.handler }

func (s *RuntimeServer) serveCall(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx := requestContext(r)
	if r.Method != http.MethodPost {
		s.logOutcome(ctx, rpcMethodUnknown, outcomeInvalidRequest, slog.LevelWarn, startedAt)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var envelope rpcEnvelope
	if err := decodeRPCJSON(r.Body, &envelope); err != nil {
		s.logOutcome(ctx, rpcMethodUnknown, outcomeInvalidRequest, slog.LevelWarn, startedAt)
		http.Error(w, "invalid execution request", http.StatusBadRequest)
		return
	}

	var result any
	var err error
	switch envelope.Method {
	case rpcOpen:
		var request OpenRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authority.ValidatePermission(ctx, request.Grant, request.Scope, rpcOpen)
		}
		if err == nil {
			result, err = s.runtime.Open(ctx, request)
		}
	case rpcQuery:
		var request QueryRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcQuery)
		}
		if err == nil {
			result, err = s.runtime.Query(ctx, request)
		}
	case rpcFetch:
		var request FetchRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcFetch)
		}
		if err == nil {
			result, err = s.runtime.Fetch(ctx, request)
		}
	case rpcExecute:
		var request ExecuteRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcExecute)
		}
		if err == nil {
			result, err = s.runtime.Execute(ctx, request)
		}
	case rpcTransactionStatus:
		var request SessionRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcTransactionStatus)
		}
		if err == nil {
			result, err = s.runtime.TransactionStatus(ctx, request)
		}
	case rpcSetTransactionMode:
		var request transactionModeRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcSetTransactionMode)
		}
		if err == nil {
			result, err = s.runtime.SetTransactionMode(ctx, request.SessionRequest, request.Mode)
		}
	case rpcCommit:
		var request SessionRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcCommit)
		}
		if err == nil {
			result, err = s.runtime.Commit(ctx, request)
		}
	case rpcRollback:
		var request SessionRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcRollback)
		}
		if err == nil {
			result, err = s.runtime.Rollback(ctx, request)
		}
	case rpcCancel:
		var request SessionRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcCancel)
		}
		if err == nil {
			err = s.runtime.Cancel(ctx, request)
		}
	case rpcClose:
		var request CloseRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcClose)
		}
		if err == nil {
			err = s.runtime.Close(ctx, request)
		}
	case rpcSession:
		var request sessionLookupRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authority.ValidatePermission(ctx, request.Grant, Scope{}, rpcSession)
		}
		if err == nil {
			var found bool
			var info SessionInfo
			info, found, err = s.runtime.Session(ctx, request.Handle)
			result = struct {
				Info  SessionInfo `json:"info"`
				Found bool        `json:"found"`
			}{Info: info, Found: found}
		}
	case rpcSessions:
		var request sessionsRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authority.ValidatePermission(ctx, request.Grant, request.Scope, rpcSessions)
		}
		if err == nil {
			var sessions []SessionInfo
			sessions, err = s.runtime.Sessions(ctx, request.Scope)
			result = sessionsResponse{Sessions: sessions}
		}
	case rpcCapabilities:
		var request SessionRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, rpcCapabilities)
		}
		if err == nil {
			result, err = s.runtime.Capabilities(ctx, request)
		}
	case rpcSchemaDirectory, rpcSchemaObjects, rpcSchemaRelationships, rpcSchemaDefinition:
		var request SchemaRequest
		if err = decodePayload(envelope.Payload, &request); err == nil {
			err = s.authorizeHandle(ctx, request.Handle, request.Grant, envelope.Method)
		}
		if err == nil {
			switch envelope.Method {
			case rpcSchemaDirectory:
				result, err = s.runtime.SchemaDirectory(ctx, request)
			case rpcSchemaObjects:
				result, err = s.runtime.SchemaObjects(ctx, request)
			case rpcSchemaRelationships:
				result, err = s.runtime.SchemaRelationships(ctx, request)
			case rpcSchemaDefinition:
				result, err = s.runtime.SchemaDefinition(ctx, request)
			}
		}
	default:
		err = fmt.Errorf("unknown execution method %q", envelope.Method)
	}
	err = writeRPCResponse(w, result, err)

	method := safeRPCMethod(envelope.Method)
	outcome, level := rpcOutcome(err)
	if method == rpcMethodUnknown {
		outcome, level = outcomeUnknownMethod, slog.LevelWarn
	}
	s.logOutcome(ctx, method, outcome, level, startedAt)
}

func (s *RuntimeServer) serveStream(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	ctx := requestContext(r)
	if r.Method != http.MethodPost {
		s.logOutcome(ctx, rpcStream, outcomeInvalidRequest, slog.LevelWarn, startedAt)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request streamRequest
	if err := decodeRPCJSON(r.Body, &request); err != nil {
		s.logOutcome(ctx, rpcStream, outcomeInvalidRequest, slog.LevelWarn, startedAt)
		http.Error(w, "invalid execution stream request", http.StatusBadRequest)
		return
	}
	if err := s.authorizeHandle(ctx, request.Handle, request.Grant, rpcStream); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		writeRPCResponse(w, nil, err)
		outcome, level := rpcOutcome(err)
		s.logOutcome(ctx, rpcStream, outcome, level, startedAt)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Add("Trailer", "X-SQLWarden-Stream-Rows")
	w.Header().Add("Trailer", "X-SQLWarden-Stream-Bytes")
	w.Header().Add("Trailer", "X-SQLWarden-Stream-Error")
	result, err := s.runtime.Stream(ctx, request.SessionRequest, w, exports.StreamOptions{
		Format: request.Options.Format, SQL: request.Options.SQL,
		MaxBytes: request.Options.MaxBytes, PageSize: request.Options.PageSize,
	})
	w.Header().Set("X-SQLWarden-Stream-Rows", strconv.FormatInt(result.Rows, 10))
	w.Header().Set("X-SQLWarden-Stream-Bytes", strconv.FormatInt(result.Bytes, 10))
	if err != nil {
		encoded, _ := json.Marshal(newRPCError(err))
		w.Header().Set("X-SQLWarden-Stream-Error", base64.RawURLEncoding.EncodeToString(encoded))
	}
	outcome, level := rpcOutcome(err)
	s.logOutcome(ctx, rpcStream, outcome, level, startedAt)
}

// requestContext attaches the caller's correlation ID, when valid, so runtime
// logs for this request share it.
func requestContext(r *http.Request) context.Context {
	ctx := r.Context()
	if requestID := observability.NormalizeRequestID(r.Header.Get(observability.RequestIDHeader)); requestID != "" {
		ctx = observability.WithRequestID(ctx, requestID)
	}
	return ctx
}

func (s *RuntimeServer) logOutcome(ctx context.Context, method, outcome string, level slog.Level, startedAt time.Time) {
	attrs := []slog.Attr{
		slog.String("rpc_method", method),
		slog.String("outcome", outcome),
		slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
	}
	s.logger.LogAttrs(ctx, level, "execution rpc completed", appendRequestID(ctx, attrs)...)
}

// authorizeHandle authenticates the grant before it reads session state, so an
// unauthenticated caller cannot use the reply to probe which opaque handles
// exist. The signature covers the scope, so comparing the session scope against
// the signed scope is equivalent to validating against the session scope.
func (s *RuntimeServer) authorizeHandle(ctx context.Context, handle SessionHandle, grant Grant, permission string) error {
	if err := s.authority.ValidatePermission(ctx, grant, grant.Scope, permission); err != nil {
		return err
	}
	info, found, err := s.runtime.Session(ctx, handle)
	if err != nil {
		return err
	}
	if !found {
		return sessionLost(nil)
	}
	if info.Scope != grant.Scope {
		return grantInvalid(nil)
	}
	return nil
}

func decodeRPCJSON(reader io.Reader, target any) error {
	limited := &io.LimitedReader{R: reader, N: maxRPCBodyBytes + 1}
	payload, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(payload) > maxRPCBodyBytes {
		return errors.New("execution request exceeds maximum size")
	}
	return decodeStrictJSON(payload, target)
}

func decodePayload(payload json.RawMessage, target any) error {
	if len(payload) > maxRPCBodyBytes {
		return invalidPayloadError{err: errors.New("execution payload exceeds maximum size")}
	}
	if err := decodeStrictJSON(payload, target); err != nil {
		return invalidPayloadError{err: err}
	}
	return nil
}

func decodeStrictJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("execution request contains trailing JSON")
	}
	return nil
}

// writeRPCResponse writes the RPC reply and returns the error the caller
// observed, which differs from err only when the result could not be encoded.
func writeRPCResponse(w http.ResponseWriter, value any, err error) error {
	w.Header().Set("Content-Type", "application/json")
	response := rpcResponse{Error: newRPCError(err)}
	if err == nil && value != nil {
		var marshalErr error
		response.Payload, marshalErr = json.Marshal(value)
		if marshalErr != nil {
			response.Error = newRPCError(marshalErr)
			err = errServerEncoding
		}
	}
	_ = json.NewEncoder(w).Encode(response)
	return err
}
