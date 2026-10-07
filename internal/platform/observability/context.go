package observability

import "context"

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID stores the request correlation ID on a context so lower layers
// can include it in operational logs without depending on HTTP-specific code.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestID returns the request correlation ID previously attached to ctx.
func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}
