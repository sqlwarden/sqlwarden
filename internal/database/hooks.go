package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/observability"
	"github.com/uptrace/bun"
)

// slowQueryDetectorHook is a [bun.QueryHook] that detects slow queries and logs them.
// Slow queries are defined as queries that take longer than a specified threshold in milliseconds to execute.
type slowQueryDetectorHook struct {
	threshold    int64        // threshold in milliseconds
	includeQuery bool         // include SQL text only when query logging is explicitly enabled
	logger       *slog.Logger // logger to use for logging slow queries
}

// AfterQuery implements [bun.QueryHook].
func (s *slowQueryDetectorHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	duration := time.Since(event.StartTime)
	if duration.Milliseconds() <= s.threshold {
		return
	}

	attrs := []slog.Attr{slog.String("duration", duration.String())}
	if requestID := observability.RequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if s.includeQuery {
		attrs = append(attrs, slog.String("query", event.Query))
	}

	s.logger.LogAttrs(ctx, slog.LevelWarn, "slow query detected", attrs...)
}

// BeforeQuery implements [bun.QueryHook].
func (s *slowQueryDetectorHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	return ctx
}

var _ bun.QueryHook = (*slowQueryDetectorHook)(nil) // enforce that slowQueryDetectorHook implements bun.QueryHook

// debugQueryLoggerHook is a [bun.QueryHook] that logs all queries for debugging purposes.
type debugQueryLoggerHook struct {
	logger *slog.Logger // logger to use for logging queries
}

// AfterQuery implements [bun.QueryHook].
func (d *debugQueryLoggerHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	var rowsAffected int64
	if event.Result != nil {
		rowsAffected, _ = event.Result.RowsAffected()
	}

	attrs := []slog.Attr{
		slog.String("duration", time.Since(event.StartTime).String()),
		slog.String("query", event.Query),
		slog.Int64("rows_affected", rowsAffected),
	}
	if requestID := observability.RequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if event.Err != nil {
		attrs = append(attrs, slog.Any("error", event.Err))
	}
	d.logger.LogAttrs(ctx, slog.LevelDebug, "executed query", attrs...)
}

// BeforeQuery implements [bun.QueryHook].
func (d *debugQueryLoggerHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	return ctx
}

var _ bun.QueryHook = (*debugQueryLoggerHook)(nil) // enforce that debugQueryLoggerHook implements bun.QueryHook
