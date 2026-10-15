//go:build enterprise

// Package ee supplies the Enterprise edition implementation.
package ee

import (
	"github.com/sqlwarden/ee/assets"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/edition"
)

const FeatureAuditTamperEvidence = "audit.tamper_evidence"

// auditCoreMigrationFloor is the core migration that created audit_events,
// which the enterprise audit stream chains.
const auditCoreMigrationFloor = 44

type Enterprise struct{ audit AuditOptions }

func New() *Enterprise                              { return &Enterprise{} }
func NewWithAudit(options AuditOptions) *Enterprise { return &Enterprise{audit: options} }
func (*Enterprise) Name() string                    { return edition.EnterpriseName }
func (*Enterprise) Licenser() edition.Licenser      { return edition.NoopLicenser{} }
func (e *Enterprise) Modules() []edition.Module {
	return []edition.Module{auditModule{options: e.audit}}
}

type auditModule struct{ options AuditOptions }

func (auditModule) Name() string    { return "audit" }
func (auditModule) Feature() string { return FeatureAuditTamperEvidence }
func (m auditModule) Register(registrar *edition.Registrar) error {
	if err := registrar.Migration(edition.EmbeddedMigration{
		StreamName:    "enterprise",
		Files:         assets.EmbeddedFiles,
		PostgresPath:  "migrations_postgres",
		SQLitePath:    "migrations_sqlite",
		HistoryTable:  "schema_migrations_ee",
		Compatibility: edition.CoreCompatibility{Minimum: auditCoreMigrationFloor},
	}); err != nil {
		return err
	}
	deps := registrar.Deps()
	return registrar.DecorateAudit(func(core audit.Writer) audit.Writer {
		return NewAuditWriter(core, deps, m.options)
	})
}

var _ edition.Edition = (*Enterprise)(nil)
