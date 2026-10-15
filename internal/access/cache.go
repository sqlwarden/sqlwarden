package access

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sqlwarden/internal/platform/invalidate"
)

// authorizationCacheTTL bounds how long an entry survives a lost
// invalidation message.
const authorizationCacheTTL = 15 * time.Minute

// Keys published on invalidate.TopicAuthz:
//
//	org:<orgID>                          org policy
//	principals:<orgID>:<accountID>       account principals within an org
//	ancestry:<resourceType>:<resourceID> resource ancestry
//
// A subscriber that cannot parse a key clears the whole cache, so a key
// shape it does not understand never leaves a stale grant behind.
const (
	authzKeyOrg        = "org"
	authzKeyPrincipals = "principals"
	authzKeyAncestry   = "ancestry"
)

func orgPolicyKey(orgID int64) string {
	return authzKeyOrg + ":" + strconv.FormatInt(orgID, 10)
}

func principalsKey(orgID, accountID int64) string {
	return authzKeyPrincipals + ":" + strconv.FormatInt(orgID, 10) + ":" + strconv.FormatInt(accountID, 10)
}

func ancestryKey(resourceType string, resourceID int64) string {
	return authzKeyAncestry + ":" + resourceType + ":" + strconv.FormatInt(resourceID, 10)
}

// AncestorLevel is one row from resource_hierarchy for a given child.
type AncestorLevel struct {
	ResourceType string
	ResourceID   int64
}

// OrgPolicy is the complete cached policy for one org.
type OrgPolicy struct {
	// rolePermissions[roleID][permission] = true
	rolePermissions map[int64]map[string]bool
	// roleScopeTypes[roleID] = scope_type
	roleScopeTypes map[int64]string
	// roleBindings[(type,id)] = []binding
	roleBindings map[resourceKey][]cachedRoleBinding
	loadedAt     time.Time
}

type resourceKey struct {
	typ string
	id  int64
}

type cachedRoleBinding struct {
	roleID      int64
	subjectType string
	subjectID   int64
	expiresAt   *time.Time
}

// AuthorizationCache defines the typed caching contract for the Enforcer.
// It is intentionally domain-specific rather than a generic byte cache because
// permission checks are hot-path and should avoid serialization overhead.
type AuthorizationCache interface {
	GetOrgPolicy(orgID int64) (*OrgPolicy, bool)
	SetOrgPolicy(orgID int64, policy *OrgPolicy)
	InvalidateOrgPolicy(orgID int64)

	GetPrincipals(orgID, accountID int64) (Principals, bool)
	SetPrincipals(orgID, accountID int64, principals Principals)
	InvalidatePrincipals(orgID, accountID int64)

	GetAncestry(resourceType string, resourceID int64) ([]AncestorLevel, bool)
	SetAncestry(resourceType string, resourceID int64, ancestry []AncestorLevel)
	InvalidateAncestry(resourceType string, resourceID int64)
}

// MemoryAuthorizationCache is an in-process RBAC cache with TTL-based expiry.
// When built with an Invalidator, every Invalidate* call is also broadcast on
// invalidate.TopicAuthz so caches in other processes drop the same entry.
type MemoryAuthorizationCache struct {
	mu          sync.RWMutex
	orgPolicies map[int64]*cachedOrgPolicy
	principals  map[principalKey]*cachedPrincipals
	ancestries  map[resourceKey]*cachedAncestry

	now    func() time.Time
	bus    invalidate.Invalidator
	logger *slog.Logger
}

type cachedOrgPolicy struct {
	policy    *OrgPolicy
	expiresAt time.Time
}

type principalKey struct {
	orgID     int64
	accountID int64
}

type cachedPrincipals struct {
	principals Principals
	expiresAt  time.Time
}

type Principals struct {
	OrgID              int64
	TeamIDs            []int64
	WorkspaceMemberIDs []int64
	OrgMember          bool
}

type cachedAncestry struct {
	levels    []AncestorLevel
	expiresAt time.Time
}

// NewMemoryAuthorizationCache returns an empty in-process RBAC cache that is
// not connected to an invalidation bus.
func NewMemoryAuthorizationCache() *MemoryAuthorizationCache {
	return newMemoryAuthorizationCache(nil, nil, time.Now)
}

// newMemoryAuthorizationCache subscribes the cache to bus when bus is non-nil.
// The subscription lives as long as the bus, which the composition root owns.
func newMemoryAuthorizationCache(bus invalidate.Invalidator, logger *slog.Logger, now func() time.Time) *MemoryAuthorizationCache {
	if logger == nil {
		logger = slog.Default()
	}
	if now == nil {
		now = time.Now
	}
	c := &MemoryAuthorizationCache{
		orgPolicies: make(map[int64]*cachedOrgPolicy),
		principals:  make(map[principalKey]*cachedPrincipals),
		ancestries:  make(map[resourceKey]*cachedAncestry),
		now:         now,
		bus:         bus,
		logger:      logger,
	}
	if bus != nil {
		bus.Subscribe(invalidate.TopicAuthz, c.applyInvalidation)
	}
	return c
}

func (c *MemoryAuthorizationCache) GetOrgPolicy(orgID int64) (*OrgPolicy, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.orgPolicies[orgID]
	if !ok || c.now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.policy, true
}

func (c *MemoryAuthorizationCache) SetOrgPolicy(orgID int64, policy *OrgPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgPolicies[orgID] = &cachedOrgPolicy{policy: policy, expiresAt: c.now().Add(authorizationCacheTTL)}
}

func (c *MemoryAuthorizationCache) InvalidateOrgPolicy(orgID int64) {
	c.dropOrgPolicy(orgID)
	c.publish(orgPolicyKey(orgID))
}

func (c *MemoryAuthorizationCache) GetPrincipals(orgID, accountID int64) (Principals, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.principals[principalKey{orgID, accountID}]
	if !ok || c.now().After(entry.expiresAt) {
		return Principals{}, false
	}
	return entry.principals, true
}

func (c *MemoryAuthorizationCache) SetPrincipals(orgID, accountID int64, principals Principals) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.principals[principalKey{orgID, accountID}] = &cachedPrincipals{principals: principals, expiresAt: c.now().Add(authorizationCacheTTL)}
}

func (c *MemoryAuthorizationCache) InvalidatePrincipals(orgID, accountID int64) {
	c.dropPrincipals(orgID, accountID)
	c.publish(principalsKey(orgID, accountID))
}

func (c *MemoryAuthorizationCache) GetAncestry(resourceType string, resourceID int64) ([]AncestorLevel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.ancestries[resourceKey{resourceType, resourceID}]
	if !ok || c.now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.levels, true
}

func (c *MemoryAuthorizationCache) SetAncestry(resourceType string, resourceID int64, ancestry []AncestorLevel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ancestries[resourceKey{resourceType, resourceID}] = &cachedAncestry{levels: ancestry, expiresAt: c.now().Add(authorizationCacheTTL)}
}

func (c *MemoryAuthorizationCache) InvalidateAncestry(resourceType string, resourceID int64) {
	c.dropAncestry(resourceType, resourceID)
	c.publish(ancestryKey(resourceType, resourceID))
}

// publish runs after the local delete, so a transport failure still leaves
// this process consistent and only remote caches fall back to the TTL.
func (c *MemoryAuthorizationCache) publish(key string) {
	if c.bus == nil {
		return
	}
	if err := c.bus.Publish(context.Background(), invalidate.TopicAuthz, key); err != nil {
		c.logger.Warn("authorization cache invalidation publish failed",
			slog.String("topic", string(invalidate.TopicAuthz)),
			slog.String("key", key),
			slog.Any("error", err),
		)
	}
}

func (c *MemoryAuthorizationCache) applyInvalidation(key string) {
	kind, rest, _ := strings.Cut(key, ":")
	switch kind {
	case authzKeyOrg:
		if orgID, err := strconv.ParseInt(rest, 10, 64); err == nil {
			c.dropOrgPolicy(orgID)
			return
		}
	case authzKeyPrincipals:
		if org, account, ok := strings.Cut(rest, ":"); ok {
			orgID, orgErr := strconv.ParseInt(org, 10, 64)
			accountID, accountErr := strconv.ParseInt(account, 10, 64)
			if orgErr == nil && accountErr == nil {
				c.dropPrincipals(orgID, accountID)
				return
			}
		}
	case authzKeyAncestry:
		if i := strings.LastIndex(rest, ":"); i > 0 {
			if resourceID, err := strconv.ParseInt(rest[i+1:], 10, 64); err == nil {
				c.dropAncestry(rest[:i], resourceID)
				return
			}
		}
	}
	c.logger.Warn("unrecognized authorization invalidation key; clearing authorization cache",
		slog.String("topic", string(invalidate.TopicAuthz)),
		slog.String("key", key),
	)
	c.clear()
}

func (c *MemoryAuthorizationCache) dropOrgPolicy(orgID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.orgPolicies, orgID)
}

func (c *MemoryAuthorizationCache) dropPrincipals(orgID, accountID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.principals, principalKey{orgID, accountID})
}

func (c *MemoryAuthorizationCache) dropAncestry(resourceType string, resourceID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.ancestries, resourceKey{resourceType, resourceID})
}

func (c *MemoryAuthorizationCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.orgPolicies)
	clear(c.principals)
	clear(c.ancestries)
}
