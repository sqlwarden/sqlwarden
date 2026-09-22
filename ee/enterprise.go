// Package ee is the Enterprise Edition composition root. Core packages never
// import this tree; an Enterprise binary opts into it explicitly.
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

// Enterprise composes licensed Enterprise modules and their decorators.
type Enterprise struct {
	licensed     bool
	modules      []edition.Module
	entitlements edition.Entitlements
}

// New constructs and validates the Enterprise edition for cfg.
func New(cfg config.Config) (*Enterprise, error) {
	candidate := newEnterprise(cfg.Edition.LicenseFile != "")
	if err := edition.Validate(candidate, cfg); err != nil {
		return nil, err
	}
	return candidate, nil
}

func newEnterprise(licensed bool) *Enterprise {
	scim := scimModule{}
	return &Enterprise{
		licensed: licensed,
		modules:  []edition.Module{scim},
		entitlements: edition.Entitlements{
			CapabilitySCIM: true,
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

// AuditWriter implements [edition.Edition].
func (*Enterprise) AuditWriter(core audit.Writer) audit.Writer { return core }

// Entitlements implements [edition.Edition].
func (e *Enterprise) Entitlements() edition.Entitlements { return e.entitlements.Clone() }

// Modules implements [edition.Edition].
func (e *Enterprise) Modules() []edition.Module { return append([]edition.Module(nil), e.modules...) }

var (
	_ edition.Edition        = (*Enterprise)(nil)
	_ access.PolicyEvaluator = licensedPolicyEvaluator{}
)
