package database

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestMigrateUpFromVersion40DropsSchemaSyncJobsAndLazyThreshold(t *testing.T) {
	ctx := context.Background()
	db, err := New("sqlite", filepath.Join(t.TempDir(), "version-40.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Nil(t, err)
	defer db.Close()

	assert.Nil(t, db.MigrateUp())
	_, err = db.ExecContext(ctx, `
		ALTER TABLE instance_settings ADD COLUMN schema_lazy_threshold INTEGER NOT NULL DEFAULT 500;
		ALTER TABLE instance_settings ADD COLUMN schema_snapshot_freshness_seconds INTEGER NOT NULL DEFAULT 86400;
		ALTER TABLE organization_runtime_settings ADD COLUMN schema_snapshot_freshness_seconds INTEGER;
		ALTER TABLE instance_settings ADD COLUMN personal_spaces_enabled BOOLEAN NOT NULL DEFAULT TRUE;
		DROP TABLE audit_events;
	`)
	assert.Nil(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO jobs (id, type, visibility, status, run_at) VALUES
		('legacy-sync', 'schema_sync', 'internal', 'queued', CURRENT_TIMESTAMP),
		('other-job', 'query_export', 'user', 'queued', CURRENT_TIMESTAMP)`)
	assert.Nil(t, err)
	_, err = db.ExecContext(ctx, "UPDATE schema_migrations SET version = 40, dirty = 0")
	assert.Nil(t, err)

	assert.Nil(t, db.MigrateUp())

	var syncJobs, otherJobs int
	assert.Nil(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE type = 'schema_sync'").Scan(&syncJobs))
	assert.Nil(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jobs WHERE id = 'other-job'").Scan(&otherJobs))
	assert.Equal(t, syncJobs, 0)
	assert.Equal(t, otherJobs, 1)

	var columns int
	assert.Nil(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('instance_settings') WHERE name = 'schema_lazy_threshold'").Scan(&columns))
	assert.Equal(t, columns, 0)
}
