package edition

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
)

func TestValidateRejectsEditionMismatch(t *testing.T) {
	cfg := config.Default()
	cfg.Edition.Name = config.EditionEnterprise
	cfg.Edition.LicenseFile = "/run/secrets/license"
	if err := Validate(NewCommunity(), cfg); err == nil {
		t.Fatal("expected a community composition with enterprise configuration to fail")
	}
}

func TestMigrateRejectsIncompatibleCoreRange(t *testing.T) {
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "edition.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	candidate := migrationEdition{stream: incompatibleStream{}}
	err = Migrate(context.Background(), candidate, db)
	if err == nil {
		t.Fatal("expected incompatible edition migrations to fail")
	}
}

type migrationEdition struct {
	stream MigrationStream
}

func (migrationEdition) Name() string { return config.EditionCommunity }
func (migrationEdition) IdentityProvider(core identity.Provider, _ Dependencies) identity.Provider {
	return core
}
func (migrationEdition) PolicyEvaluator(core access.PolicyEvaluator, _ Dependencies) access.PolicyEvaluator {
	return core
}
func (migrationEdition) AuditWriter(core audit.Writer) audit.Writer { return core }
func (migrationEdition) Entitlements() Entitlements                 { return Entitlements{} }
func (e migrationEdition) Modules() []Module                        { return []Module{migrationModule{stream: e.stream}} }

type migrationModule struct{ stream MigrationStream }

func (migrationModule) Name() string                          { return "migration-test" }
func (migrationModule) Capabilities() Entitlements            { return Entitlements{} }
func (migrationModule) Validate(config.Config) error          { return nil }
func (m migrationModule) MigrationStreams() []MigrationStream { return []MigrationStream{m.stream} }

type incompatibleStream struct{}

func (incompatibleStream) Name() string { return "incompatible" }
func (incompatibleStream) CoreCompatibility() CoreCompatibility {
	return CoreCompatibility{Minimum: database.CoreMigrationVersion + 1, Maximum: database.CoreMigrationVersion + 1}
}
func (incompatibleStream) Migrate(context.Context, *database.DB) error { return nil }
