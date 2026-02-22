package database

import (
	"context"
	"os"

	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestNew(t *testing.T) {
	t.Run("Creates DB connection pool", func(t *testing.T) {
		dsn := os.Getenv("TEST_DB_DSN")

		if dsn == "" {
			dsn = "user:pass@localhost:5432/db?sslmode=disable"
		}

		db, err := New(dsn)
		assert.Nil(t, err)
		assert.NotNil(t, db)
		assert.NotNil(t, db.Pool)
		defer db.Close()

		err = db.Ping(context.Background())
		assert.Nil(t, err)

		assert.Equal(t, int32(25), db.Stat().MaxConns())
	})

	t.Run("Fails with invalid DSN", func(t *testing.T) {
		dsn := "fake_user:fake_pass@localhost:5432/fake_db"

		db, err := New(dsn)
		assert.NotNil(t, err)
		assert.Nil(t, db)
	})
}

func TestMigrateUp(t *testing.T) {
	t.Run("Applies all up migrations", func(t *testing.T) {
		db := newTestDB(t)

		err := db.MigrateUp()
		assert.Nil(t, err)

		var version int
		err = db.QueryRow(context.Background(), "SELECT version FROM schema_migrations LIMIT 1").Scan(&version)
		assert.Nil(t, err)
		assert.True(t, version > 0)
	})
}
