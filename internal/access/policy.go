package access

import "context"

// PolicyEvaluator is the authorization decision seam decorated by editions.
// The concrete Enforcer implements it directly. Decorators may only preserve
// or restrict core grants; they must never grant a permission core denied.
type PolicyEvaluator interface {
	Can(
		ctx context.Context,
		accountID, orgID int64,
		ownerType, resourceType string,
		resourceID int64,
		permission string,
	) bool
	EffectivePermissions(
		ctx context.Context,
		accountID, orgID int64,
		ownerType, resourceType string,
		resourceID int64,
	) ([]string, error)
}

var _ PolicyEvaluator = (*Enforcer)(nil)
