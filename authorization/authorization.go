// Package authorization defines the public authorization decision boundary.
package authorization

import "context"

// Request describes one permission check against a concrete resource.
type Request struct {
	AccountID    int64
	OrgID        int64
	OwnerType    string
	ResourceType string
	ResourceID   int64
	Permission   string
}

// Decision is the result of an authorization check. Code is stable and safe to
// expose to clients; Message is an optional user-facing explanation.
type Decision struct {
	Allowed bool
	Code    string
	Message string
}

// Authorizer evaluates Community RBAC eligibility.
type Authorizer interface {
	Authorize(context.Context, Request) Decision
	EffectivePermissions(context.Context, Request) ([]string, error)
}

// Constraint may further restrict a Community-approved request. It is never
// called for requests denied by the base authorizer.
type Constraint interface {
	Evaluate(context.Context, Request) Decision
}

// ConstraintFunc adapts a function to Constraint.
type ConstraintFunc func(context.Context, Request) Decision

func (fn ConstraintFunc) Evaluate(ctx context.Context, request Request) Decision {
	return fn(ctx, request)
}

// Constrained preserves the base decision and applies an optional additional constraint.
type Constrained struct {
	Base       Authorizer
	Constraint Constraint
}

func (a Constrained) Authorize(ctx context.Context, request Request) Decision {
	decision := a.Base.Authorize(ctx, request)
	if !decision.Allowed || a.Constraint == nil {
		return decision
	}
	return a.Constraint.Evaluate(ctx, request)
}

func (a Constrained) EffectivePermissions(ctx context.Context, request Request) ([]string, error) {
	return a.Base.EffectivePermissions(ctx, request)
}
