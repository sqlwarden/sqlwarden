package ee

import (
	"context"
	"fmt"

	eeassets "github.com/sqlwarden/ee/assets"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/edition"
)

// CapabilitySCIM identifies directory provisioning support in capability
// responses. It never substitutes for backend authorization.
const CapabilitySCIM = "identity.scim"

type scimModule struct{}

func (scimModule) Name() string { return "scim" }

func (scimModule) Capabilities() edition.Entitlements {
	return edition.Entitlements{CapabilitySCIM: true}
}

func (scimModule) Validate(cfg config.Config) error {
	if cfg.Edition.Name != config.EditionEnterprise {
		return fmt.Errorf("SCIM module requires enterprise edition")
	}
	if cfg.Edition.LicenseFile == "" {
		return fmt.Errorf("SCIM module requires an enterprise license source")
	}
	return nil
}

func (scimModule) MigrationStreams() []edition.MigrationStream {
	return []edition.MigrationStream{scimMigrationStream{}}
}

type scimMigrationStream struct{}

func (scimMigrationStream) Name() string { return "ee-scim" }

func (scimMigrationStream) CoreCompatibility() edition.CoreCompatibility {
	return edition.CoreCompatibility{Minimum: 39, Maximum: 39}
}

func (scimMigrationStream) Migrate(_ context.Context, db *database.DB) error {
	return db.MigrateStream(
		eeassets.EmbeddedFiles,
		"migrations_postgres",
		"migrations_sqlite",
		"ee_schema_migrations",
	)
}

var (
	_ edition.Module          = scimModule{}
	_ edition.MigratingModule = scimModule{}
	_ edition.MigrationStream = scimMigrationStream{}
)
