package authorization_test

import (
	"context"
	"testing"

	"github.com/sqlwarden/authorization"
)

type fixedAuthorizer struct{ allowed bool }

func (a fixedAuthorizer) Authorize(context.Context, authorization.Request) authorization.Decision {
	return authorization.Decision{Allowed: a.allowed}
}
func (a fixedAuthorizer) EffectivePermissions(context.Context, authorization.Request) ([]string, error) {
	return []string{"conn:dql"}, nil
}

func TestConstrainedAuthorizationPreservesBaseDenials(t *testing.T) {
	constraintCalled := false
	authorizer := authorization.Constrained{
		Base: fixedAuthorizer{allowed: false},
		Constraint: authorization.ConstraintFunc(func(context.Context, authorization.Request) authorization.Decision {
			constraintCalled = true
			return authorization.Decision{Allowed: true}
		}),
	}
	if authorizer.Authorize(context.Background(), authorization.Request{}).Allowed {
		t.Fatal("base denial was elevated")
	}
	if constraintCalled {
		t.Fatal("constraint must not run after a base denial")
	}
}

func TestConstrainedAuthorizationMayRestrictBaseAllow(t *testing.T) {
	authorizer := authorization.Constrained{
		Base: fixedAuthorizer{allowed: true},
		Constraint: authorization.ConstraintFunc(func(context.Context, authorization.Request) authorization.Decision {
			return authorization.Decision{Code: "approval_required"}
		}),
	}
	decision := authorizer.Authorize(context.Background(), authorization.Request{})
	if decision.Allowed || decision.Code != "approval_required" {
		t.Fatalf("decision = %#v", decision)
	}
}
