package database

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func newRawPostgresTestDSN(t *testing.T) string {
	t.Helper()

	dbName := fmt.Sprintf("internal_database_raw_%d", atomic.AddUint64(&pgTestDBCounter, 1))

	pgTemplateCloneMu.Lock()
	_, err := pgAdminDB.ExecContext(context.Background(), fmt.Sprintf("CREATE DATABASE %s", dbName))
	pgTemplateCloneMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	dsn := trimPostgresScheme(dsnWithDatabase("postgres://"+pgAdminDSN, dbName))
	t.Cleanup(func() {
		pgTemplateCloneMu.Lock()
		defer pgTemplateCloneMu.Unlock()
		_, _ = pgAdminDB.ExecContext(context.Background(),
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()",
			dbName)
		_, err := pgAdminDB.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		if err != nil {
			t.Error(err)
		}
	})

	return dsn
}

func TestNew(t *testing.T) {
	drivers := []struct {
		name   string
		driver string
	}{
		{"PostgreSQL", "postgres"},
		{"SQLite", "sqlite"},
	}

	for _, tc := range drivers {
		t.Run(tc.name+": Creates DB connection pool", func(t *testing.T) {
			dsn := "test.db"
			if tc.driver == "sqlite" {
				defer os.Remove(dsn)
			} else {
				dsn = newRawPostgresTestDSN(t)
			}

			db, err := New(tc.driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
			assert.Nil(t, err)
			assert.NotNil(t, db)
			assert.NotNil(t, db.DB)
			defer db.Close()

			err = db.Ping()
			assert.Nil(t, err)

			expectedMaxOpen := 25
			if tc.driver == "sqlite" {
				expectedMaxOpen = 1
			}
			assert.Equal(t, expectedMaxOpen, db.DB.DB.Stats().MaxOpenConnections)
		})

		t.Run("Fails with invalid DSN", func(t *testing.T) {
			dsn := "fake_user:fake_pass@127.0.0.1:1/fake_db?sslmode=disable"

			db, err := New("postgres", dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
			assert.NotNil(t, err)
			assert.Nil(t, db)
		})
	}
}

func TestMigrateUp(t *testing.T) {
	drivers := []struct {
		name   string
		driver string
	}{
		{"PostgreSQL", "postgres"},
		{"SQLite", "sqlite"},
	}

	for _, tc := range drivers {
		t.Run(tc.name+": Applies all up migrations", func(t *testing.T) {
			dsn := "test.db"
			if tc.driver == "sqlite" {
				defer os.Remove(dsn)
			} else {
				dsn = newRawPostgresTestDSN(t)
			}

			db, err := New(tc.driver, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
			assert.Nil(t, err)
			assert.NotNil(t, db)
			defer db.Close()

			err = db.MigrateUp()
			assert.Nil(t, err)

			var version int
			err = db.NewSelect().ColumnExpr("version").TableExpr("schema_migrations").Limit(1).Scan(context.Background(), &version)
			assert.Nil(t, err)
			assert.True(t, version > 0)
		})
	}
}
