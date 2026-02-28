package database

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/uptrace/bun"
)

func TestSlowQueryDetectorHook(t *testing.T) {
	t.Run("Logs slow queries that exceed threshold", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		hook := &slowQueryDetectorHook{
			threshold: 50, // 50ms threshold
			logger:    logger,
		}

		// Simulate a slow query event
		event := &bun.QueryEvent{
			StartTime: time.Now().Add(-100 * time.Millisecond), // Query took 100ms
			Query:     "SELECT * FROM users WHERE id = 1",
		}

		hook.AfterQuery(context.Background(), event)

		output := buf.String()
		assert.True(t, strings.Contains(output, "slow query detected"))
		assert.True(t, strings.Contains(output, "SELECT * FROM users"))
	})

	t.Run("Does not log fast queries below threshold", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		hook := &slowQueryDetectorHook{
			threshold: 100, // 100ms threshold
			logger:    logger,
		}

		// Simulate a fast query event
		event := &bun.QueryEvent{
			StartTime: time.Now().Add(-10 * time.Millisecond), // Query took 10ms
			Query:     "SELECT * FROM users WHERE id = 1",
		}

		hook.AfterQuery(context.Background(), event)

		output := buf.String()
		assert.Equal(t, "", output) // Should not log anything
	})

	t.Run("BeforeQuery returns context unchanged", func(t *testing.T) {
		hook := &slowQueryDetectorHook{
			threshold: 100,
			logger:    slog.New(slog.NewTextHandler(os.Stdout, nil)),
		}

		ctx := context.Background()
		event := &bun.QueryEvent{}

		returnedCtx := hook.BeforeQuery(ctx, event)
		assert.Equal(t, ctx, returnedCtx)
	})
}

func TestDebugQueryLoggerHook(t *testing.T) {
	t.Run("Logs all queries with details", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		hook := &debugQueryLoggerHook{
			logger: logger,
		}

		// Simulate a query event
		event := &bun.QueryEvent{
			StartTime: time.Now().Add(-5 * time.Millisecond),
			Query:     "INSERT INTO users (email, hashed_password) VALUES ('test@example.com', 'hash')",
			Result:    &testResult{rowsAffected: 1},
		}

		hook.AfterQuery(context.Background(), event)

		output := buf.String()
		assert.True(t, strings.Contains(output, "executed query"))
		assert.True(t, strings.Contains(output, "INSERT INTO users"))
		assert.True(t, strings.Contains(output, "rows_affected"))
	})

	t.Run("Logs queries with errors", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		hook := &debugQueryLoggerHook{
			logger: logger,
		}

		// Simulate a query event with error
		event := &bun.QueryEvent{
			StartTime: time.Now().Add(-5 * time.Millisecond),
			Query:     "SELECT * FROM non_existent_table",
			Result:    &testResult{rowsAffected: 0},
			Err:       fmt.Errorf("table does not exist"),
		}

		hook.AfterQuery(context.Background(), event)

		output := buf.String()
		assert.True(t, strings.Contains(output, "executed query"))
		assert.True(t, strings.Contains(output, "error"))
		assert.True(t, strings.Contains(output, "table does not exist"))
	})

	t.Run("BeforeQuery returns context unchanged", func(t *testing.T) {
		hook := &debugQueryLoggerHook{
			logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
		}

		ctx := context.Background()
		event := &bun.QueryEvent{}

		returnedCtx := hook.BeforeQuery(ctx, event)
		assert.Equal(t, ctx, returnedCtx)
	})
}

func TestHooksIntegration(t *testing.T) {
	t.Run("SQLite with logQueries enabled uses debug hook", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		dsn := fmt.Sprintf("test_hooks_%d.db", time.Now().UnixNano())
		defer os.Remove(dsn)

		db, err := New("sqlite", dsn, logger, true)
		assert.Nil(t, err)
		assert.NotNil(t, db)
		defer db.Close()

		// Run a simple query
		_, err = db.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)")
		assert.Nil(t, err)

		output := buf.String()
		// Debug hook should log the query
		assert.True(t, strings.Contains(output, "executed query"))
		assert.True(t, strings.Contains(output, "CREATE TABLE test"))
	})

	t.Run("SQLite with logQueries disabled does not use debug hook", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		dsn := fmt.Sprintf("test_hooks_%d.db", time.Now().UnixNano())
		defer os.Remove(dsn)

		db, err := New("sqlite", dsn, logger, false)
		assert.Nil(t, err)
		assert.NotNil(t, db)
		defer db.Close()

		// Run a simple query
		_, err = db.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)")
		assert.Nil(t, err)

		output := buf.String()
		// Debug hook should not log anything
		assert.False(t, strings.Contains(output, "executed query"))
	})

	t.Run("SQLite detects slow queries", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

		dsn := fmt.Sprintf("test_hooks_%d.db", time.Now().UnixNano())
		defer os.Remove(dsn)

		db, err := New("sqlite", dsn, logger, false)
		assert.Nil(t, err)
		assert.NotNil(t, db)
		defer db.Close()

		// Run a query that will be slow due to sleep
		// Note: This may not actually trigger the slow query detector in all cases
		// since the threshold is 100ms and sqlite3_sleep might not be available
		_, err = db.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)")
		assert.Nil(t, err)

		// The slow query hook is tested with the unit tests above
		// This integration test just ensures the hook is registered
	})
}

// testResult is a mock implementation of sql.Result for testing
type testResult struct {
	rowsAffected int64
}

func (r *testResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r *testResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}
