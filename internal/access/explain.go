package access

import (
	"context"
	"time"
)

// Grant is one role binding that produced an authorization decision, with the
// expiry recorded on the binding row. Core RBAC ignores expiry; an edition
// decorator uses it to restrict a grant core already made.
type Grant struct {
	BindingID    int64
	RoleID       int64
	SubjectType  string
	SubjectID    int64
	ResourceType string
	ResourceID   int64
	ExpiresAt    *time.Time
}

// Expired reports whether the binding expired at now.
func (g Grant) Expired(now time.Time) bool {
	return g.ExpiresAt != nil && !now.Before(*g.ExpiresAt)
}

// GrantExplainer reports the role bindings that grant a permission. It exists
// so an edition decorator can restrict a decision based on binding metadata
// core does not evaluate, without re-implementing the RBAC walk.
//
// An empty result with a nil error means no role binding produced the
// decision, which is the case for owner-scoped personal space resources.
type GrantExplainer interface {
	ExplainGrant(
		ctx context.Context,
		accountID, orgID int64,
		ownerType, resourceType string,
		resourceID int64,
		permission string,
	) ([]Grant, error)
}

// ExplainGrant implements [GrantExplainer].
func (e *Enforcer) ExplainGrant(ctx context.Context,
	accountID, orgID int64,
	ownerType, resourceType string, resourceID int64,
	permission string,
) ([]Grant, error) {
	if ownerType == "space" {
		return nil, nil
	}

	principals, err := e.principalsFor(ctx, orgID, accountID)
	if err != nil {
		return nil, err
	}
	ancestors, err := e.ancestryFor(ctx, ownerType, resourceType, resourceID, orgID)
	if err != nil {
		return nil, err
	}
	policy, err := e.orgPolicy(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var grants []Grant
	for _, level := range ancestors {
		key := resourceKey{level.ResourceType, level.ResourceID}
		for _, rb := range policy.roleBindings[key] {
			if !matchesPrincipal(rb.subjectType, rb.subjectID, accountID, principals) {
				continue
			}
			if !policy.rolePermissions[rb.roleID][permission] {
				continue
			}
			grants = append(grants, Grant{
				BindingID:    rb.bindingID,
				RoleID:       rb.roleID,
				SubjectType:  rb.subjectType,
				SubjectID:    rb.subjectID,
				ResourceType: level.ResourceType,
				ResourceID:   level.ResourceID,
				ExpiresAt:    rb.expiresAt,
			})
		}
	}
	return grants, nil
}

var _ GrantExplainer = (*Enforcer)(nil)
