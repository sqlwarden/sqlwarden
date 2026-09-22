package ee

import (
	"context"

	eeassets "github.com/sqlwarden/ee/assets"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
)

// enterpriseMigrationStream is the single ordered schema history the
// Enterprise edition owns. Every Enterprise table lives in it, numbered in one
// sequence and recorded in one history table, so no two modules can each
// believe they own the same schema.
type enterpriseMigrationStream struct{}

func (enterpriseMigrationStream) Name() string { return "enterprise" }

func (enterpriseMigrationStream) CoreCompatibility() edition.CoreCompatibility {
	return edition.CoreCompatibility{Minimum: 40, Maximum: 40}
}

func (enterpriseMigrationStream) Migrate(_ context.Context, db *database.DB) error {
	return db.MigrateStream(
		eeassets.EmbeddedFiles,
		"migrations_postgres",
		"migrations_sqlite",
		"ee_schema_migrations",
	)
}

var _ edition.MigrationStream = enterpriseMigrationStream{}
