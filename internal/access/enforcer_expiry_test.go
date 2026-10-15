package access

import (
	"context"
	"testing"
	"time"
)

func cachedTestEnforcer(now *time.Time, expiresAt *time.Time) *Enforcer {
	cache := NewMemoryAuthorizationCache()
	cache.SetPrincipals(1, 2, Principals{OrgID: 1, OrgMember: true})
	cache.SetOrgPolicy(1, &OrgPolicy{
		rolePermissions: map[int64]map[string]bool{10: {PermOrgRead: true}},
		roleScopeTypes:  map[int64]string{10: "org"},
		roleBindings: map[resourceKey][]cachedRoleBinding{
			{typ: "org", id: 1}: {{
				roleID: 10, subjectType: SubjectTypeAccount, subjectID: 2, expiresAt: expiresAt,
			}},
		},
	})
	return &Enforcer{cache: cache, now: func() time.Time { return *now }}
}

func TestRoleBindingExpiryIsReevaluatedAfterPolicyIsCached(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	expires := now.Add(time.Minute)
	enforcer := cachedTestEnforcer(&now, &expires)
	ctx := context.Background()
	if !enforcer.Can(ctx, 2, 1, "org", "org", 1, PermOrgRead) {
		t.Fatal("future binding was denied")
	}
	now = expires
	if enforcer.Can(ctx, 2, 1, "org", "org", 1, PermOrgRead) {
		t.Fatal("binding at its exact expiry boundary was allowed")
	}
	permissions, err := enforcer.EffectivePermissions(ctx, 2, 1, "org", "org", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 0 {
		t.Fatalf("expired effective permissions = %v", permissions)
	}
}

func TestPermanentRoleBindingRemainsAllowed(t *testing.T) {
	now := time.Now()
	enforcer := cachedTestEnforcer(&now, nil)
	if !enforcer.Can(context.Background(), 2, 1, "org", "org", 1, PermOrgRead) {
		t.Fatal("permanent binding was denied")
	}
}
