package web

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lmittmann/tint"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/version"
	"github.com/tomasen/realip"
)

const requestIDHeader = "X-Request-ID"

type requestLogContext struct {
	RequestID     string
	AccountID     int64
	AuthSessionID string
	OrgID         int64
	OrgSlug       string
	WorkspaceID   int64
	EnvironmentID int64
	ConnectionID  int64
}

// NewLogger builds the process logger from runtime configuration.
func NewLogger(cfg Config, out io.Writer) (*slog.Logger, error) {
	level, err := parseLogLevel(cfg.Log.Level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch cfg.Log.Format {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(out, opts)
	case LogFormatText:
		handler = tint.NewHandler(out, &tint.Options{Level: level})
	default:
		return nil, fmt.Errorf("unsupported log format: %s", cfg.Log.Format)
	}

	return slog.New(handler).With(
		"service", "sqlwarden",
		"version", version.Get(),
	), nil
}

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case LogLevelDebug:
		return slog.LevelDebug, nil
	case LogLevelInfo:
		return slog.LevelInfo, nil
	case LogLevelWarn:
		return slog.LevelWarn, nil
	case LogLevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %s", level)
	}
}

func accessLogLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func requestAttrs(r *http.Request) []slog.Attr {
	meta := contextGetRequestLogContext(r)
	attrs := []slog.Attr{
		slog.String("method", r.Method),
		slog.String("path", requestPath(r)),
		slog.String("proto", r.Proto),
		slog.String("remote_ip", realip.FromRequest(r)),
	}
	if route := routePattern(r); route != "" {
		attrs = append(attrs, slog.String("route", route))
	}
	if meta != nil && meta.RequestID != "" {
		attrs = append(attrs, slog.String("id", meta.RequestID))
	}
	if ua := r.UserAgent(); ua != "" {
		attrs = append(attrs, slog.String("user_agent", ua))
	}
	return attrs
}

func resourceAttrs(r *http.Request) []slog.Attr {
	meta := contextGetRequestLogContext(r)
	if meta == nil {
		return nil
	}

	attrs := make([]slog.Attr, 0, 8)
	if meta.AccountID != 0 {
		attrs = append(attrs, slog.Int64("account_id", meta.AccountID))
	}
	if meta.AuthSessionID != "" {
		attrs = append(attrs, slog.String("auth_session_id", meta.AuthSessionID))
	}
	if meta.OrgID != 0 {
		attrs = append(attrs, slog.Int64("org_id", meta.OrgID))
	}
	if meta.OrgSlug != "" {
		attrs = append(attrs, slog.String("org_slug", meta.OrgSlug))
	}
	if meta.WorkspaceID != 0 {
		attrs = append(attrs, slog.Int64("workspace_id", meta.WorkspaceID))
	}
	if meta.EnvironmentID != 0 {
		attrs = append(attrs, slog.Int64("environment_id", meta.EnvironmentID))
	}
	if meta.ConnectionID != 0 {
		attrs = append(attrs, slog.Int64("connection_id", meta.ConnectionID))
	}
	return attrs
}

func accessLogAttrs(r *http.Request, mw *response.MetricsResponseWriter, duration time.Duration) []slog.Attr {
	return []slog.Attr{
		slog.Group("request", attrsToAny(requestAttrs(r))...),
		slog.Group("response",
			slog.Int("status", mw.StatusCode),
			slog.Int("size", mw.BytesCount),
			slog.Int64("duration_ms", duration.Milliseconds()),
		),
		slog.Group("resource", attrsToAny(resourceAttrs(r))...),
	}
}

// logInfo records a successful domain event with the same request/resource
// correlation fields used by access and error logs.
func (app *application) logInfo(r *http.Request, message string, attrs ...slog.Attr) {
	base := []slog.Attr{
		slog.Group("request", attrsToAny(requestAttrs(r))...),
		slog.Group("resource", attrsToAny(resourceAttrs(r))...),
	}
	base = append(base, attrs...)
	app.logger.LogAttrs(r.Context(), slog.LevelInfo, message, base...)
}

// logWarn records a high-signal blocked or degraded domain event with request
// correlation. Routine 4xx responses are still covered by access logs.
func (app *application) logWarn(r *http.Request, message string, attrs ...slog.Attr) {
	base := []slog.Attr{
		slog.Group("request", attrsToAny(requestAttrs(r))...),
		slog.Group("resource", attrsToAny(resourceAttrs(r))...),
	}
	base = append(base, attrs...)
	app.logger.LogAttrs(r.Context(), slog.LevelWarn, message, base...)
}

func attrsToAny(attrs []slog.Attr) []any {
	out := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, attr)
	}
	return out
}

func requestPath(r *http.Request) string {
	if r.URL == nil {
		return ""
	}
	path := r.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func routePattern(r *http.Request) string {
	routeCtx := chi.RouteContext(r.Context())
	if routeCtx == nil {
		return ""
	}
	return routeCtx.RoutePattern()
}
