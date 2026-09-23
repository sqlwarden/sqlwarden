package execution

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sqlwarden/internal/observability"
)

// Option configures optional dependencies of execution components.
type Option func(*componentOptions)

type componentOptions struct {
	logger *slog.Logger
}

// WithLogger sets the operational logger. Components constructed without it
// discard their logs rather than falling back to the process default logger.
func WithLogger(logger *slog.Logger) Option {
	return func(options *componentOptions) {
		if logger != nil {
			options.logger = logger
		}
	}
}

func applyOptions(opts []Option) componentOptions {
	options := componentOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.logger == nil {
		options.logger = slog.New(slog.DiscardHandler)
	}
	return options
}

// Execution logs carry only these low-cardinality categories. Raw errors never
// appear because target drivers and transports may embed SQL, bind values,
// connection strings, or network addresses in their messages.
const (
	outcomeSuccess          = "success"
	outcomeInvalidRequest   = "invalid_request"
	outcomeUnknownMethod    = "unknown_method"
	outcomeUnauthorized     = "unauthorized"
	outcomeInternalError    = "internal_error"
	outcomeApplicationError = "application_error"

	rpcMethodUnknown = "unknown"
)

// Transport failure categories recorded by WorkerRuntime.
const (
	transportCategoryCanceled         = "canceled"
	transportCategoryRequestEncoding  = "request_encoding"
	transportCategoryRouting          = "routing"
	transportCategoryProtocolMismatch = "protocol_mismatch"
	transportCategoryUnreachable      = "unreachable"
	transportCategoryRemoteStatus     = "remote_status"
	transportCategoryResponseDecoding = "response_decoding"
	transportCategoryStreamCopy       = "stream_copy"
	transportCategoryClientWrite      = "client_write"
)

// Session open failure stages recorded by LocalRuntime.
const (
	openStageCredentialProvider = "credential_provider"
	openStageCredentials        = "credentials"
	openStageTargetPolicy       = "target_policy"
	openStageTunnel             = "tunnel"
	openStageDriver             = "driver"
	openStageConnect            = "connect"
	openStageSessionPool        = "session_pool"
	openStageDirectory          = "directory"
)

var knownRPCMethods = map[string]struct{}{
	rpcOpen: {}, rpcQuery: {}, rpcFetch: {}, rpcExecute: {}, rpcTransactionStatus: {},
	rpcSetTransactionMode: {}, rpcCommit: {}, rpcRollback: {}, rpcCancel: {}, rpcClose: {},
	rpcSession: {}, rpcSessions: {}, rpcCapabilities: {}, rpcSchemaDirectory: {},
	rpcSchemaObjects: {}, rpcSchemaRelationships: {}, rpcSchemaDefinition: {}, rpcStream: {},
}

// safeRPCMethod keeps caller-controlled method names out of logs.
func safeRPCMethod(method string) string {
	if _, ok := knownRPCMethods[method]; ok {
		return method
	}
	return rpcMethodUnknown
}

// invalidPayloadError marks a request that failed strict decoding while
// preserving the decoder message returned to the caller.
type invalidPayloadError struct{ err error }

func (e invalidPayloadError) Error() string { return e.err.Error() }
func (e invalidPayloadError) Unwrap() error { return e.err }

// errServerEncoding marks a runtime result the server could not encode.
var errServerEncoding = errors.New("execution response could not be encoded")

// rpcOutcome maps a server-side RPC error to its outcome category and level.
// Stage failures inside the runtime (credentials, tunnels, directory) are
// logged at their own severity by the runtime that owns them, so the server
// records them as outcomes without escalating.
func rpcOutcome(err error) (string, slog.Level) {
	if err == nil {
		return outcomeSuccess, slog.LevelDebug
	}
	var invalid invalidPayloadError
	switch {
	case errors.As(err, &invalid):
		return outcomeInvalidRequest, slog.LevelWarn
	case errors.Is(err, errServerEncoding):
		return outcomeInternalError, slog.LevelError
	case errors.Is(err, ErrGrantInvalid):
		return outcomeUnauthorized, slog.LevelWarn
	}
	wire := newRPCError(err)
	if wire.Kind != "" {
		if _, ok := redactedRPCKinds[wire.Kind]; ok {
			return wire.Kind, slog.LevelInfo
		}
		return wire.Kind, slog.LevelDebug
	}
	switch wire.Code {
	case FailureSessionLost, FailureCursorLost, FailureTransactionLost, FailureExecutionOutcomeUnknown:
		return string(wire.Code), slog.LevelInfo
	case "":
		return outcomeApplicationError, slog.LevelDebug
	default:
		return string(wire.Code), slog.LevelDebug
	}
}

// contextOr reports caller cancellation instead of category, because a
// cancelled operation is not an operational fault.
func contextOr(ctx context.Context, category string) string {
	if ctx.Err() != nil {
		return transportCategoryCanceled
	}
	return category
}

func contextLevel(ctx context.Context, level slog.Level) slog.Level {
	if ctx.Err() != nil {
		return slog.LevelDebug
	}
	return level
}

func appendRequestID(ctx context.Context, attrs []slog.Attr) []slog.Attr {
	if requestID := observability.RequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	return attrs
}
