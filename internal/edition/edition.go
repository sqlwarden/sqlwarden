// Package edition defines the extension boundary between SQLWarden core and
// edition-specific modules. Core code depends only on these contracts.
package edition

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
)

// Entitlements is the immutable capability view exposed to clients. A
// capability controls presentation only; backend authorization remains
// authoritative for every operation.
type Entitlements map[string]bool

// Enabled reports whether a capability is enabled.
func (e Entitlements) Enabled(capability string) bool {
	return e[capability]
}

// Clone returns a copy safe for callers to mutate.
func (e Entitlements) Clone() Entitlements {
	clone := make(Entitlements, len(e))
	for capability, enabled := range e {
		clone[capability] = enabled
	}
	return clone
}

// Module is one bounded edition feature. Validate sees only bootstrap
// configuration, not the application service graph.
type Module interface {
	Name() string
	Capabilities() Entitlements
	Validate(config.Config) error
}

// CoreCompatibility declares the inclusive core migration versions an edition
// migration stream supports.
type CoreCompatibility struct {
	Minimum uint
	Maximum uint
}

// MigrationStream is an independently ordered edition migration stream.
// Streams use their own migration history table and run only after core
// migrations have reached a compatible version.
type MigrationStream interface {
	Name() string
	CoreCompatibility() CoreCompatibility
	Migrate(ctx context.Context, db *database.DB) error
}

// MigratingModule optionally contributes edition-owned migrations.
type MigratingModule interface {
	Module
	MigrationStreams() []MigrationStream
}

// Dependencies is the fixed set of core capabilities an edition decorator may
// build on. It is deliberately a closed struct rather than a service locator:
// every field is a named core contract, so a decorator cannot reach arbitrary
// application state and the compiler reports any capability an edition needs
// that core does not yet offer.
type Dependencies struct {
	// DB is the SQLWarden metadata database. Edition modules read and write
	// only their own edition-owned tables through it.
	DB *database.DB
	// Grants explains which role bindings produced a core grant, so a decorator
	// can restrict a decision using binding metadata core ignores.
	Grants access.GrantExplainer
	// Now is the clock used for time-dependent decisions. Tests substitute it.
	Now func() time.Time
	// Logger receives edition decision logs. It is never nil.
	Logger *slog.Logger
	// Federation verifies SAML and OIDC credentials before an Enterprise
	// identity decorator maps the directory subject to a core account.
	Federation identity.FederationVerifier
}

// Normalize fills the optional fields of d with safe defaults.
func (d Dependencies) Normalize() Dependencies {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return d
}

// Edition composes extension decorators without exposing the application
// container. Decorator order is defined by each concrete edition.
type Edition interface {
	Name() string
	IdentityProvider(core identity.Provider, deps Dependencies) identity.Provider
	PolicyEvaluator(core access.PolicyEvaluator, deps Dependencies) access.PolicyEvaluator
	AuditWriter(core audit.Writer) audit.Writer
	Entitlements() Entitlements
	Modules() []Module
}

// Community is the default edition. It preserves core providers unchanged and
// enables no edition-only capabilities.
type Community struct{}

// NewCommunity returns the default Community edition.
func NewCommunity() Community { return Community{} }

// Name implements [Edition].
func (Community) Name() string { return config.EditionCommunity }

// IdentityProvider implements [Edition].
func (Community) IdentityProvider(core identity.Provider, _ Dependencies) identity.Provider {
	return core
}

// PolicyEvaluator implements [Edition].
func (Community) PolicyEvaluator(core access.PolicyEvaluator, _ Dependencies) access.PolicyEvaluator {
	return core
}

// AuditWriter implements [Edition].
func (Community) AuditWriter(core audit.Writer) audit.Writer { return core }

// Entitlements implements [Edition].
func (Community) Entitlements() Entitlements { return Entitlements{} }

// Modules implements [Edition].
func (Community) Modules() []Module { return nil }

// Validate checks edition identity, module uniqueness, module configuration,
// and capability consistency before any process starts.
func Validate(candidate Edition, cfg config.Config) error {
	if candidate == nil {
		return fmt.Errorf("edition is required")
	}
	if candidate.Name() != cfg.Edition.Name {
		return fmt.Errorf("configured edition %q does not match composed edition %q", cfg.Edition.Name, candidate.Name())
	}

	moduleNames := make(map[string]struct{})
	entitlements := candidate.Entitlements()
	for _, module := range candidate.Modules() {
		if module == nil {
			return fmt.Errorf("edition %q contains a nil module", candidate.Name())
		}
		name := module.Name()
		if name == "" {
			return fmt.Errorf("edition %q contains a module with no name", candidate.Name())
		}
		if _, exists := moduleNames[name]; exists {
			return fmt.Errorf("edition %q contains duplicate module %q", candidate.Name(), name)
		}
		moduleNames[name] = struct{}{}
		if err := module.Validate(cfg); err != nil {
			return fmt.Errorf("validate edition module %q: %w", name, err)
		}
		for capability, enabled := range module.Capabilities() {
			if enabled && !entitlements.Enabled(capability) {
				return fmt.Errorf("module %q enables capability %q missing from edition entitlements", name, capability)
			}
		}
	}
	return nil
}

// Migrate applies module migration streams after core migrations.
func Migrate(ctx context.Context, candidate Edition, db *database.DB) error {
	for _, module := range candidate.Modules() {
		migrating, ok := module.(MigratingModule)
		if !ok {
			continue
		}
		for _, stream := range migrating.MigrationStreams() {
			compatibility := stream.CoreCompatibility()
			if database.CoreMigrationVersion < compatibility.Minimum || database.CoreMigrationVersion > compatibility.Maximum {
				return fmt.Errorf(
					"edition migration stream %q supports core migrations %d..%d, running core version is %d",
					stream.Name(), compatibility.Minimum, compatibility.Maximum, database.CoreMigrationVersion,
				)
			}
			if err := stream.Migrate(ctx, db); err != nil {
				return fmt.Errorf("migrate edition stream %q: %w", stream.Name(), err)
			}
		}
	}
	return nil
}
