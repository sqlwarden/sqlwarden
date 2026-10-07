// Package edition defines the build-selected product extension boundary.
package edition

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/jobs"
	"github.com/uptrace/bun"
)

const (
	CommunityName  = "community"
	EnterpriseName = "enterprise"
)

type State string

const (
	StateAvailable  State = "available"
	StateUpgrade    State = "upgrade"
	StateUnlicensed State = "unlicensed"
)

// Feature is core-owned catalog data. Optional presentation targets remain nil
// until a real product surface exists.
type Feature struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	DocsURL     *string `json:"docs_url"`
	Navigation  *string `json:"navigation"`
}

var Catalog = []Feature{{
	Key:         "audit.tamper_evidence",
	Label:       "Tamper-evident audit",
	Description: "Hash-chained audit evidence with retention, optional signing, and a SIEM export hook.",
}}

type Capability struct {
	Feature
	State State `json:"state"`
}

type Capabilities struct {
	Edition  string       `json:"edition"`
	Features []Capability `json:"features"`
}

func (c Capabilities) Clone() Capabilities {
	return Capabilities{Edition: c.Edition, Features: append([]Capability(nil), c.Features...)}
}

func (c Capabilities) Available(key string) bool {
	for _, feature := range c.Features {
		if feature.Key == key {
			return feature.State == StateAvailable
		}
	}
	return false
}

type License struct {
	Features  map[string]bool
	ExpiresAt *time.Time
}

type Licenser interface {
	Check(context.Context) (License, error)
}

type LicenserFunc func(context.Context) (License, error)

func (f LicenserFunc) Check(ctx context.Context) (License, error) { return f(ctx) }

// NoopLicenser licenses every feature in the core catalog.
type NoopLicenser struct{}

func (NoopLicenser) Check(context.Context) (License, error) {
	features := make(map[string]bool, len(Catalog))
	for _, feature := range Catalog {
		features[feature.Key] = true
	}
	return License{Features: features}, nil
}

type Edition interface {
	Name() string
	Licenser() Licenser
	Modules() []Module
}

type Module interface {
	Name() string
	Feature() string
	Register(*Registrar) error
}

// CoreCompatibility bounds the core migration versions a stream was written
// against. A zero Maximum means no upper bound.
type CoreCompatibility struct {
	Minimum uint
	Maximum uint
}

func (c CoreCompatibility) supports(version uint) bool {
	return version >= c.Minimum && (c.Maximum == 0 || version <= c.Maximum)
}

type MigrationStream interface {
	Name() string
	CoreCompatibility() CoreCompatibility
	Migrate(context.Context, *database.DB) error
}

// EmbeddedMigration is the standard independently-versioned migration stream.
type EmbeddedMigration struct {
	StreamName    string
	Files         fs.FS
	PostgresPath  string
	SQLitePath    string
	HistoryTable  string
	Compatibility CoreCompatibility
}

func (m EmbeddedMigration) Name() string                         { return m.StreamName }
func (m EmbeddedMigration) CoreCompatibility() CoreCompatibility { return m.Compatibility }
func (m EmbeddedMigration) Migrate(_ context.Context, db *database.DB) error {
	return db.MigrateStream(m.Files, m.PostgresPath, m.SQLitePath, m.HistoryTable)
}

// Dependencies contains only the ports available to edition modules. It never
// exposes the web or app layers.
type Dependencies struct {
	SQL         bun.IDB
	AuditEvents audit.Reader
	Logger      *slog.Logger
	Now         func() time.Time
}

type AuditDecorator func(audit.Writer) audit.Writer
type PolicyDecorator func(access.PolicyEvaluator) access.PolicyEvaluator
type AuthenticatorDecorator func(identity.Authenticator) identity.Authenticator
type CredentialDecorator func(access.CredentialInfo) access.CredentialInfo

type Route struct {
	Module  string
	Handler http.Handler
}

type Setting struct{ Key string }

// Registrar collects typed contributions in module order. Register may only
// be called while a module is being composed.
type Registrar struct {
	deps                    Dependencies
	module                  string
	auditDecorators         []AuditDecorator
	policyDecorators        []PolicyDecorator
	authenticatorDecorators []AuthenticatorDecorator
	credentialDecorators    []CredentialDecorator
	routes                  []Route
	jobs                    []jobs.Definition
	settings                []Setting
	migrations              []MigrationStream
	authenticators          []identity.Authenticator
	methods                 []identity.Method
	factors                 []identity.Factor
	factorPolicies          []identity.FactorPolicy
	signInPolicies          []identity.SignInPolicy
	requestPolicies         []identity.RequestPolicy
	conditions              []access.Condition
	posture                 []identity.PostureProvider
	keys                    map[string]struct{}
}

func newRegistrar(deps Dependencies) *Registrar {
	return &Registrar{deps: deps, keys: make(map[string]struct{})}
}

// Deps returns the ports a module may use to build its contributions.
func (r *Registrar) Deps() Dependencies { return r.deps }

func (r *Registrar) unique(kind, name string) error {
	if name == "" {
		return fmt.Errorf("edition: %s name is required", kind)
	}
	key := kind + ":" + name
	if _, exists := r.keys[key]; exists {
		return fmt.Errorf("edition: duplicate %s %q", kind, name)
	}
	r.keys[key] = struct{}{}
	return nil
}

func (r *Registrar) DecorateAudit(d AuditDecorator) error {
	if d == nil {
		return errors.New("edition: nil audit decorator")
	}
	r.auditDecorators = append(r.auditDecorators, d)
	return nil
}
func (r *Registrar) DecoratePolicy(d PolicyDecorator) error {
	if d == nil {
		return errors.New("edition: nil policy decorator")
	}
	r.policyDecorators = append(r.policyDecorators, d)
	return nil
}
func (r *Registrar) DecorateIdentity(d AuthenticatorDecorator) error {
	if d == nil {
		return errors.New("edition: nil identity decorator")
	}
	r.authenticatorDecorators = append(r.authenticatorDecorators, d)
	return nil
}
func (r *Registrar) DecorateCredential(d CredentialDecorator) error {
	if d == nil {
		return errors.New("edition: nil credential decorator")
	}
	r.credentialDecorators = append(r.credentialDecorators, d)
	return nil
}
func (r *Registrar) Route(handler http.Handler) error {
	if handler == nil {
		return errors.New("edition: nil route handler")
	}
	if err := r.unique("route", r.module); err != nil {
		return err
	}
	r.routes = append(r.routes, Route{Module: r.module, Handler: handler})
	return nil
}
func (r *Registrar) Job(def jobs.Definition) error {
	if err := r.unique("job", def.Type); err != nil {
		return err
	}
	r.jobs = append(r.jobs, def)
	return nil
}
func (r *Registrar) Setting(setting Setting) error {
	if err := r.unique("setting", setting.Key); err != nil {
		return err
	}
	r.settings = append(r.settings, setting)
	return nil
}
func (r *Registrar) Migration(stream MigrationStream) error {
	if stream == nil {
		return errors.New("edition: nil migration stream")
	}
	if err := r.unique("migration", stream.Name()); err != nil {
		return err
	}
	r.migrations = append(r.migrations, stream)
	return nil
}
func (r *Registrar) Authenticator(value identity.Authenticator) {
	r.authenticators = append(r.authenticators, value)
}
func (r *Registrar) Method(value identity.Method) { r.methods = append(r.methods, value) }
func (r *Registrar) Factor(value identity.Factor) { r.factors = append(r.factors, value) }
func (r *Registrar) FactorPolicy(value identity.FactorPolicy) {
	r.factorPolicies = append(r.factorPolicies, value)
}
func (r *Registrar) SignInPolicy(value identity.SignInPolicy) {
	r.signInPolicies = append(r.signInPolicies, value)
}
func (r *Registrar) RequestPolicy(value identity.RequestPolicy) {
	r.requestPolicies = append(r.requestPolicies, value)
}
func (r *Registrar) Condition(value access.Condition) { r.conditions = append(r.conditions, value) }
func (r *Registrar) PostureProvider(value identity.PostureProvider) {
	r.posture = append(r.posture, value)
}

type Composition struct {
	name         string
	capabilities Capabilities
	registrar    *Registrar
}

// Compose checks the license and registers each licensed module with deps.
func Compose(ctx context.Context, candidate Edition, deps Dependencies) (*Composition, error) {
	if candidate == nil {
		return nil, errors.New("edition is required")
	}
	if candidate.Name() == "" {
		return nil, errors.New("edition name is required")
	}
	licenser := candidate.Licenser()
	if licenser == nil {
		return nil, fmt.Errorf("edition %q has no licenser", candidate.Name())
	}
	license, err := licenser.Check(ctx)
	if err != nil {
		return nil, fmt.Errorf("check %s license: %w", candidate.Name(), err)
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	if license.ExpiresAt != nil && !now().Before(*license.ExpiresAt) {
		license.Features = nil
	}

	states := make(map[string]State, len(Catalog))
	known := make(map[string]bool, len(Catalog))
	for _, feature := range Catalog {
		states[feature.Key] = StateUpgrade
		known[feature.Key] = true
	}
	registrar := newRegistrar(deps)
	modules := make(map[string]struct{})
	moduleFeatures := make(map[string]string)
	for _, module := range candidate.Modules() {
		if module == nil {
			return nil, fmt.Errorf("edition %q contains a nil module", candidate.Name())
		}
		name, feature := module.Name(), module.Feature()
		if name == "" {
			return nil, fmt.Errorf("edition %q contains a module with no name", candidate.Name())
		}
		if _, exists := modules[name]; exists {
			return nil, fmt.Errorf("edition %q contains duplicate module %q", candidate.Name(), name)
		}
		modules[name] = struct{}{}
		if !known[feature] {
			return nil, fmt.Errorf("module %q references unknown feature %q", name, feature)
		}
		if owner, exists := moduleFeatures[feature]; exists {
			return nil, fmt.Errorf("modules %q and %q reference the same feature %q", owner, name, feature)
		}
		moduleFeatures[feature] = name
		if !license.Features[feature] {
			states[feature] = StateUnlicensed
			continue
		}
		registrar.module = name
		if err := module.Register(registrar); err != nil {
			return nil, fmt.Errorf("register edition module %q: %w", name, err)
		}
		states[feature] = StateAvailable
	}
	registrar.module = ""
	capabilities := Capabilities{Edition: candidate.Name(), Features: make([]Capability, 0, len(Catalog))}
	for _, feature := range Catalog {
		capabilities.Features = append(capabilities.Features, Capability{Feature: feature, State: states[feature.Key]})
	}
	sort.Slice(capabilities.Features, func(i, j int) bool { return capabilities.Features[i].Key < capabilities.Features[j].Key })
	return &Composition{name: candidate.Name(), capabilities: capabilities, registrar: registrar}, nil
}

func (c *Composition) Name() string               { return c.name }
func (c *Composition) Capabilities() Capabilities { return c.capabilities.Clone() }
func (c *Composition) Migrations() []MigrationStream {
	return append([]MigrationStream(nil), c.registrar.migrations...)
}
func (c *Composition) Jobs() []jobs.Definition {
	return append([]jobs.Definition(nil), c.registrar.jobs...)
}
func (c *Composition) Authenticators() []identity.Authenticator {
	return append([]identity.Authenticator(nil), c.registrar.authenticators...)
}
func (c *Composition) RequestPolicies() []identity.RequestPolicy {
	return append([]identity.RequestPolicy(nil), c.registrar.requestPolicies...)
}
func (c *Composition) PostureProviders() []identity.PostureProvider {
	return append([]identity.PostureProvider(nil), c.registrar.posture...)
}

func (c *Composition) Audit(core audit.Writer) audit.Writer {
	for _, decorator := range c.registrar.auditDecorators {
		core = decorator(core)
	}
	return core
}
func (c *Composition) Policy(core access.PolicyEvaluator) access.PolicyEvaluator {
	baseline := core
	for _, decorator := range c.registrar.policyDecorators {
		core = decorator(core)
	}
	if len(c.registrar.policyDecorators) == 0 {
		return core
	}
	return restrictivePolicy{baseline: baseline, extension: core}
}

// Authenticator applies identity decorators in module registration order.
func (c *Composition) Authenticator(core identity.Authenticator) identity.Authenticator {
	for _, decorator := range c.registrar.authenticatorDecorators {
		core = decorator(core)
	}
	return core
}

// Unwired lists contribution kinds that a module registered but that the
// running core does not consume yet. The host must refuse to start with them
// rather than drop them silently.
func (c *Composition) Unwired() []string {
	r := c.registrar
	var kinds []string
	for kind, count := range map[string]int{
		"credential decorators": len(r.credentialDecorators),
		"settings":              len(r.settings),
		"methods":               len(r.methods),
		"factors":               len(r.factors),
		"factor policies":       len(r.factorPolicies),
		"sign-in policies":      len(r.signInPolicies),
		"conditions":            len(r.conditions),
	} {
		if count > 0 {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

// restrictivePolicy makes the extension seam monotonic: decorators may deny
// a core grant, but they cannot manufacture a grant that core denied.
type restrictivePolicy struct {
	baseline  access.PolicyEvaluator
	extension access.PolicyEvaluator
}

func (p restrictivePolicy) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return p.baseline.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission) &&
		p.extension.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func (p restrictivePolicy) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	baseline, err := p.baseline.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	extension, err := p.extension.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(extension))
	for _, permission := range extension {
		allowed[permission] = struct{}{}
	}
	result := baseline[:0]
	for _, permission := range baseline {
		if _, ok := allowed[permission]; ok {
			result = append(result, permission)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (c *Composition) Handler() http.Handler {
	if len(c.registrar.routes) == 0 {
		return nil
	}
	mux := http.NewServeMux()
	for _, route := range c.registrar.routes {
		prefix := "/" + route.Module
		mux.Handle(prefix, http.StripPrefix(prefix, route.Handler))
		mux.Handle(prefix+"/", http.StripPrefix(prefix, route.Handler))
	}
	return mux
}

func Migrate(ctx context.Context, db *database.DB, streams []MigrationStream) error {
	for _, stream := range streams {
		compatibility := stream.CoreCompatibility()
		if !compatibility.supports(database.CoreMigrationVersion) {
			return fmt.Errorf("edition migration stream %q supports core migrations %d..%d, running core version is %d", stream.Name(), compatibility.Minimum, compatibility.Maximum, database.CoreMigrationVersion)
		}
	}
	for _, stream := range streams {
		if err := stream.Migrate(ctx, db); err != nil {
			return fmt.Errorf("migrate edition stream %q: %w", stream.Name(), err)
		}
	}
	return nil
}
