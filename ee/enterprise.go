package ee

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/app"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/identity"
)

// Enterprise composes modules gated on a configured license source and their
// decorators. No shipped entrypoint composes it yet.
type Enterprise struct {
	licensed     bool
	modules      []edition.Module
	entitlements edition.Entitlements
	audit        AuditOptions
}

// New constructs and validates the Enterprise edition for cfg.
func New(cfg config.Config) (*Enterprise, error) {
	return NewWithAudit(cfg, AuditOptions{})
}

// NewWithAudit constructs the Enterprise edition with an audit signing hook
// and SIEM exporter supplied by the deployment. Both are optional; the audit
// pipeline runs its remaining stages without them.
func NewWithAudit(cfg config.Config, options AuditOptions) (*Enterprise, error) {
	candidate := newEnterprise(cfg.Edition.LicenseFile != "")
	candidate.audit = options
	if err := edition.Validate(candidate, cfg); err != nil {
		return nil, err
	}
	return candidate, nil
}

func newEnterprise(licensed bool) *Enterprise {
	return &Enterprise{
		licensed: licensed,
		modules:  []edition.Module{scimModule{}, auditModule{}},
		entitlements: edition.Entitlements{
			CapabilitySCIM:                true,
			CapabilityAuditTamperEvidence: true,
		},
	}
}

// Build composes an Enterprise application without exposing the application
// service graph to modules.
func Build(ctx context.Context, opts app.Options) (*app.Application, error) {
	candidate, err := New(opts.Config)
	if err != nil {
		return nil, err
	}
	opts.Edition = candidate
	return app.Build(ctx, opts)
}

// Name implements [edition.Edition].
func (*Enterprise) Name() string { return config.EditionEnterprise }

// IdentityProvider implements [edition.Edition]. The SCIM proof module is a
// transparent decorator; later federation modules are composed inside it.
func (e *Enterprise) IdentityProvider(core identity.Provider, deps edition.Dependencies) identity.Provider {
	return federationIdentityProvider{
		core:     core,
		verifier: deps.Federation,
		store:    newStore(deps.DB),
	}
}

// PolicyEvaluator implements [edition.Edition]. License denial is outermost so
// no inner policy decorator can grant access while the edition is unlicensed.
func (e *Enterprise) PolicyEvaluator(core access.PolicyEvaluator, deps edition.Dependencies) access.PolicyEvaluator {
	return newPolicyChain(core, e.licensed, deps)
}

// AuditWriter implements [edition.Edition]. The decorator writes durably
// through core first and only then adds tamper evidence and export, so an
// export outage can never cost an audit record.
func (e *Enterprise) AuditWriter(core audit.Writer, deps edition.Dependencies) audit.Writer {
	return newAuditWriter(core, deps, e.audit)
}

// Entitlements implements [edition.Edition].
func (e *Enterprise) Entitlements() edition.Entitlements { return e.entitlements.Clone() }

// Modules implements [edition.Edition].
func (e *Enterprise) Modules() []edition.Module { return append([]edition.Module(nil), e.modules...) }

// MigrationStreams implements [edition.MigratingEdition]. Enterprise tables
// share one schema history, so the stream is declared here rather than by each
// module that happens to read from it.
func (*Enterprise) MigrationStreams() []edition.MigrationStream {
	return []edition.MigrationStream{enterpriseMigrationStream{}}
}

var (
	_ edition.Edition          = (*Enterprise)(nil)
	_ edition.MigratingEdition = (*Enterprise)(nil)
	_ access.PolicyEvaluator   = licensedPolicyEvaluator{}
)
