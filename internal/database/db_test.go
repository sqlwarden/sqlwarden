package database

import (
	"context"
	"os"
	"testing"

	"github.com/sqlwarden/internal/assert"
)

func TestNew(t *testing.T) {
	drivers := []struct {
		name   string
		driver string
		dsn    string
	}{
		{"PostgreSQL", "postgres", "user:pass@localhost:5432/db?sslmode=disable"},
		{"SQLite", "sqlite", "test.db"},
	}

	for _, tc := range drivers {
		t.Run(tc.name+": Creates DB connection pool", func(t *testing.T) {
			if tc.driver == "sqlite" {
				defer os.Remove(tc.dsn)
			}

			db, err := New(tc.driver, tc.dsn)
			assert.Nil(t, err)
			assert.NotNil(t, db)
			assert.NotNil(t, db.DB)
			defer db.Close()

			err = db.Ping()
			assert.Nil(t, err)

			assert.Equal(t, 25, db.DB.DB.Stats().MaxOpenConnections)
		})

		t.Run("Fails with invalid DSN", func(t *testing.T) {
			dsn := "fake_user:fake_pass@localhost:5432/fake_db?sslmode=disable"

			db, err := New("postgres", dsn)
			assert.NotNil(t, err)
			assert.Nil(t, db)
		})
	}
}

func TestMigrateUp(t *testing.T) {
	drivers := []struct {
		name   string
		driver string
		dsn    string
	}{
		{"PostgreSQL", "postgres", "user:pass@localhost:5432/db?sslmode=disable"},
		{"SQLite", "sqlite", "test.db"},
	}

	for _, tc := range drivers {
		t.Run(tc.name+": Applies all up migrations", func(t *testing.T) {
			if tc.driver == "sqlite" {
				defer os.Remove(tc.dsn)
			}

			db := newTestDB(t, tc.driver)

			err := db.MigrateUp()
			assert.Nil(t, err)

			var version int
			err = db.NewSelect().ColumnExpr("version").TableExpr("schema_migrations").Limit(1).Scan(context.Background(), &version)
			assert.Nil(t, err)
			assert.True(t, version > 0)
		})
	}
}
