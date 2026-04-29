package access

import (
	"sync"
	"time"
)

const cacheTTL = 15 * time.Minute

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

// Cache defines the caching contract for the Enforcer.
type Cache interface {
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

// MemoryCache is an in-process cache with TTL-based expiry.
type MemoryCache struct {
	mu          sync.RWMutex
	orgPolicies map[int64]*cachedOrgPolicy
	principals  map[principalKey]*cachedPrincipals
	ancestries  map[resourceKey]*cachedAncestry
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

// NewMemoryCache returns an empty MemoryCache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		orgPolicies: make(map[int64]*cachedOrgPolicy),
		principals:  make(map[principalKey]*cachedPrincipals),
		ancestries:  make(map[resourceKey]*cachedAncestry),
	}
}

func (c *MemoryCache) GetOrgPolicy(orgID int64) (*OrgPolicy, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.orgPolicies[orgID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.policy, true
}

func (c *MemoryCache) SetOrgPolicy(orgID int64, policy *OrgPolicy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orgPolicies[orgID] = &cachedOrgPolicy{policy: policy, expiresAt: time.Now().Add(cacheTTL)}
}

func (c *MemoryCache) InvalidateOrgPolicy(orgID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.orgPolicies, orgID)
}

func (c *MemoryCache) GetPrincipals(orgID, accountID int64) (Principals, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.principals[principalKey{orgID, accountID}]
	if !ok || time.Now().After(entry.expiresAt) {
		return Principals{}, false
	}
	return entry.principals, true
}

func (c *MemoryCache) SetPrincipals(orgID, accountID int64, principals Principals) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.principals[principalKey{orgID, accountID}] = &cachedPrincipals{principals: principals, expiresAt: time.Now().Add(cacheTTL)}
}

func (c *MemoryCache) InvalidatePrincipals(orgID, accountID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.principals, principalKey{orgID, accountID})
}

func (c *MemoryCache) GetAncestry(resourceType string, resourceID int64) ([]AncestorLevel, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.ancestries[resourceKey{resourceType, resourceID}]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.levels, true
}

func (c *MemoryCache) SetAncestry(resourceType string, resourceID int64, ancestry []AncestorLevel) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ancestries[resourceKey{resourceType, resourceID}] = &cachedAncestry{levels: ancestry, expiresAt: time.Now().Add(cacheTTL)}
}

func (c *MemoryCache) InvalidateAncestry(resourceType string, resourceID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.ancestries, resourceKey{resourceType, resourceID})
}
