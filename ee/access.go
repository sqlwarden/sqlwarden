package ee

import (
	"context"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/edition"
)

// Policy layer names, ordered outermost first. The chain is composed so that
// every layer can only remove access: a layer consults the layer it wraps
// first and denies on its own rules afterwards, so no Enterprise rule can
// grant a permission core RBAC refused.
const (
	layerLicense           = "license"
	layerDenyRules         = "deny_rules"
	layerConditionalAccess = "conditional_access"
	layerJITAccess         = "jit_access"
	layerBindingExpiry     = "binding_expiry"
	layerCore              = "core"
)

// policyLayerOrder is the composed decorator order, outermost first. License
// state is outermost so an unlicensed instance never reads Enterprise rules,
// and binding expiry is innermost so it restricts exactly the core grant it
// wraps.
var policyLayerOrder = []string{
	layerLicense,
	layerDenyRules,
	layerConditionalAccess,
	layerJITAccess,
	layerBindingExpiry,
	layerCore,
}

// policyLayer is one link of the Enterprise decorator chain. It exists so the
// composed order is observable and testable rather than implied by
// construction code.
type policyLayer interface {
	access.PolicyEvaluator
	layer() string
	wrapped() access.PolicyEvaluator
}

// newPolicyChain composes the Enterprise policy decorators around core.
func newPolicyChain(core access.PolicyEvaluator, licensed bool, deps edition.Dependencies) access.PolicyEvaluator {
	deps = deps.Normalize()
	restrictions := restriction{store: newStore(deps.DB), now: deps.Now, logger: deps.Logger}

	var evaluator access.PolicyEvaluator = bindingExpiryPolicy{
		core:   core,
		grants: deps.Grants,
		now:    deps.Now,
		logger: deps.Logger,
	}
	evaluator = jitAccessPolicy{core: evaluator, restriction: restrictions}
	evaluator = conditionalAccessPolicy{core: evaluator, restriction: restrictions}
	evaluator = denyRulePolicy{core: evaluator, restriction: restrictions}
	return licensedPolicyEvaluator{core: evaluator, licensed: licensed}
}

// policyChainLayers reports the composed layer order, outermost first, ending
// at the undecorated core evaluator.
func policyChainLayers(evaluator access.PolicyEvaluator) []string {
	var layers []string
	for {
		current, ok := evaluator.(policyLayer)
		if !ok {
			return append(layers, layerCore)
		}
		layers = append(layers, current.layer())
		evaluator = current.wrapped()
	}
}

// restriction is the shared plumbing every rule-backed layer needs: the
// Enterprise tables, the clock, and a logger for fail-closed decisions.
type restriction struct {
	store  *store
	now    func() time.Time
	logger *slog.Logger
}

// unavailable reports a decision that could not be evaluated. Such a decision
// is always a denial: an Enterprise restriction that cannot be read must not
// silently disappear.
func (r restriction) unavailable(ctx context.Context, layer string, err error) bool {
	r.logger.WarnContext(ctx, "enterprise policy layer unavailable, denying",
		slog.Group("authorization", "layer", layer),
		"error", err,
	)
	return true
}

// filter keeps the permissions of a core result that also pass allow.
func filter(permissions []string, allow func(permission string) bool) []string {
	kept := permissions[:0:0]
	for _, permission := range permissions {
		if allow(permission) {
			kept = append(kept, permission)
		}
	}
	return kept
}

// licensedPolicyEvaluator denies every decision while the instance is
// unlicensed, without consulting any inner layer.
type licensedPolicyEvaluator struct {
	core     access.PolicyEvaluator
	licensed bool
}

func (p licensedPolicyEvaluator) layer() string                   { return layerLicense }
func (p licensedPolicyEvaluator) wrapped() access.PolicyEvaluator { return p.core }

func (p licensedPolicyEvaluator) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return p.licensed && p.core.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func (p licensedPolicyEvaluator) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	if !p.licensed {
		return nil, nil
	}
	return p.core.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
}

// denyRulePolicy applies explicit deny rules. An explicit deny outranks every
// grant, including one held by an organization owner.
type denyRulePolicy struct {
	core access.PolicyEvaluator
	restriction
}

func (p denyRulePolicy) layer() string                   { return layerDenyRules }
func (p denyRulePolicy) wrapped() access.PolicyEvaluator { return p.core }

func (p denyRulePolicy) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if !p.core.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission) {
		return false
	}
	return !p.deniedBy(ctx, accountID, orgID, resourceType, resourceID, permission)
}

func (p denyRulePolicy) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	permissions, err := p.core.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil || len(permissions) == 0 {
		return permissions, err
	}
	return filter(permissions, func(permission string) bool {
		return !p.deniedBy(ctx, accountID, orgID, resourceType, resourceID, permission)
	}), nil
}

func (p denyRulePolicy) deniedBy(ctx context.Context, accountID, orgID int64, resourceType string, resourceID int64, permission string) bool {
	if p.store == nil {
		return p.unavailable(ctx, layerDenyRules, errStoreUnavailable)
	}
	denied, err := p.store.denied(ctx, accountID, orgID, resourceType, resourceID, permission)
	if err != nil {
		return p.unavailable(ctx, layerDenyRules, err)
	}
	return denied
}

// conditionalAccessPolicy requires the session to have been authenticated in a
// way the organization accepts for this permission. Decision attributes are
// supplied by the transport; a context without them fails closed.
type conditionalAccessPolicy struct {
	core access.PolicyEvaluator
	restriction
}

func (p conditionalAccessPolicy) layer() string                   { return layerConditionalAccess }
func (p conditionalAccessPolicy) wrapped() access.PolicyEvaluator { return p.core }

func (p conditionalAccessPolicy) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if !p.core.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission) {
		return false
	}
	return p.satisfied(ctx, orgID, permission)
}

func (p conditionalAccessPolicy) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	permissions, err := p.core.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil || len(permissions) == 0 {
		return permissions, err
	}
	return filter(permissions, func(permission string) bool {
		return p.satisfied(ctx, orgID, permission)
	}), nil
}

func (p conditionalAccessPolicy) satisfied(ctx context.Context, orgID int64, permission string) bool {
	if p.store == nil {
		return !p.unavailable(ctx, layerConditionalAccess, errStoreUnavailable)
	}
	methods, err := p.store.requiredAuthMethods(ctx, orgID, permission)
	if err != nil {
		return !p.unavailable(ctx, layerConditionalAccess, err)
	}
	if len(methods) == 0 {
		return true
	}
	authMethod := access.AttributesFrom(ctx)[access.AttributeAuthMethod]
	for _, required := range methods {
		if required == authMethod {
			return true
		}
	}
	return false
}

// jitAccessPolicy holds back a standing grant until the account activates it.
// A permission covered by a just-in-time policy is exercisable only while an
// unexpired activation exists.
type jitAccessPolicy struct {
	core access.PolicyEvaluator
	restriction
}

func (p jitAccessPolicy) layer() string                   { return layerJITAccess }
func (p jitAccessPolicy) wrapped() access.PolicyEvaluator { return p.core }

func (p jitAccessPolicy) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if !p.core.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission) {
		return false
	}
	return p.activated(ctx, accountID, orgID, resourceType, resourceID, permission)
}

func (p jitAccessPolicy) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	permissions, err := p.core.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil || len(permissions) == 0 {
		return permissions, err
	}
	return filter(permissions, func(permission string) bool {
		return p.activated(ctx, accountID, orgID, resourceType, resourceID, permission)
	}), nil
}

func (p jitAccessPolicy) activated(ctx context.Context, accountID, orgID int64, resourceType string, resourceID int64, permission string) bool {
	if p.store == nil {
		return !p.unavailable(ctx, layerJITAccess, errStoreUnavailable)
	}
	required, err := p.store.jitRequired(ctx, orgID, resourceType, resourceID, permission)
	if err != nil {
		return !p.unavailable(ctx, layerJITAccess, err)
	}
	if !required {
		return true
	}
	active, err := p.store.jitActive(ctx, accountID, orgID, resourceType, resourceID, permission, p.now())
	if err != nil {
		return !p.unavailable(ctx, layerJITAccess, err)
	}
	return active
}

// bindingExpiryPolicy enforces the expiry recorded on a role binding. Core
// RBAC treats a binding as permanent, so this layer denies a permission whose
// every contributing binding has expired. A decision backed by no binding at
// all, such as personal-space ownership, is left untouched.
type bindingExpiryPolicy struct {
	core   access.PolicyEvaluator
	grants access.GrantExplainer
	now    func() time.Time
	logger *slog.Logger
}

func (p bindingExpiryPolicy) layer() string                   { return layerBindingExpiry }
func (p bindingExpiryPolicy) wrapped() access.PolicyEvaluator { return p.core }

func (p bindingExpiryPolicy) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if !p.core.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission) {
		return false
	}
	return p.live(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func (p bindingExpiryPolicy) EffectivePermissions(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64) ([]string, error) {
	permissions, err := p.core.EffectivePermissions(ctx, accountID, orgID, ownerType, resourceType, resourceID)
	if err != nil || len(permissions) == 0 {
		return permissions, err
	}
	return filter(permissions, func(permission string) bool {
		return p.live(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
	}), nil
}

func (p bindingExpiryPolicy) live(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	if p.grants == nil {
		p.logger.WarnContext(ctx, "enterprise policy layer unavailable, denying",
			slog.Group("authorization", "layer", layerBindingExpiry),
			"error", errGrantsUnavailable,
		)
		return false
	}
	grants, err := p.grants.ExplainGrant(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
	if err != nil {
		p.logger.WarnContext(ctx, "enterprise policy layer unavailable, denying",
			slog.Group("authorization", "layer", layerBindingExpiry),
			"error", err,
		)
		return false
	}
	if len(grants) == 0 {
		return true
	}
	now := p.now()
	for _, grant := range grants {
		if !grant.Expired(now) {
			return true
		}
	}
	return false
}

var (
	_ policyLayer = licensedPolicyEvaluator{}
	_ policyLayer = denyRulePolicy{}
	_ policyLayer = conditionalAccessPolicy{}
	_ policyLayer = jitAccessPolicy{}
	_ policyLayer = bindingExpiryPolicy{}
)
