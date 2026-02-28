package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/uptrace/bun"
)

// slowQueryDetectorHook is a [bun.QueryHook] that detects slow queries and logs them.
// Slow queries are defined as queries that take longer than a specified threshold in milliseconds to execute.
type slowQueryDetectorHook struct {
	threshold int64        // threshold in milliseconds
	logger    *slog.Logger // logger to use for logging slow queries
}

// AfterQuery implements [bun.QueryHook].
func (s *slowQueryDetectorHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	duration := time.Since(event.StartTime).Milliseconds()
	if duration > s.threshold {
		s.logger.Warn("slow query detected",
			"duration_ms", duration,
			"query", event.Query,
		)
	}
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
	rowsAffected, _ := event.Result.RowsAffected()
	d.logger.Debug("executed query",
		"duration_ms", time.Since(event.StartTime).Milliseconds(),
		"query", event.Query,
		"rows_affected", rowsAffected,
		"error", event.Err,
	)
}

// BeforeQuery implements [bun.QueryHook].
func (d *debugQueryLoggerHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	return ctx
}

var _ bun.QueryHook = (*debugQueryLoggerHook)(nil) // enforce that debugQueryLoggerHook implements bun.QueryHook
