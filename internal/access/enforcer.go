package access

import (
	_ "embed"
	"fmt"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/uptrace/bun"
)

//go:embed model.conf
var modelConfStr string

// actionOrdinal maps action names to ordinal values for implication checks.
var actionOrdinal = map[string]int{
	"connect": 1,
	"query":   2,
	"execute": 3,
	"manage":  4,
}

// PolicyEntry represents a subject-action pair in a policy.
type PolicyEntry struct {
	Subject string
	Action  string
}

// Enforcer wraps a Casbin SyncedEnforcer for RBAC policy management.
type Enforcer struct {
	e *casbin.SyncedEnforcer
}

// New creates a new Enforcer backed by the given bun.DB.
func New(db *bun.DB) (*Enforcer, error) {
	m, err := model.NewModelFromString(modelConfStr)
	if err != nil {
		return nil, fmt.Errorf("access: load model: %w", err)
	}

	adapter := newBunAdapter(db)

	e, err := casbin.NewSyncedEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("access: new enforcer: %w", err)
	}

	return &Enforcer{e: e}, nil
}

// impliedActions returns all actions that satisfy the requested action.
// Requesting "query" is satisfied by "query", "execute", or "manage".
func impliedActions(act string) []string {
	ord, ok := actionOrdinal[act]
	if !ok {
		return []string{act}
	}
	var result []string
	for a, o := range actionOrdinal {
		if o >= ord {
			result = append(result, a)
		}
	}
	return result
}

// Can checks if accountID has permission for the given action on obj in orgSlug.
func (enf *Enforcer) Can(accountID, orgSlug, obj, act string) bool {
	for _, action := range impliedActions(act) {
		ok, err := enf.e.Enforce(accountID, orgSlug, obj, action)
		if err == nil && ok {
			return true
		}
	}
	return false
}

// CanOnConnection checks if accountID can perform act on a connection,
// considering both direct connection policies and workspace-level policies.
func (enf *Enforcer) CanOnConnection(accountID, orgSlug, connID, wsID, act string) bool {
	candidates := []string{"connection:" + connID, "workspace:" + wsID}
	for _, action := range impliedActions(act) {
		for _, obj := range candidates {
			ok, err := enf.e.Enforce(accountID, orgSlug, obj, action)
			if err == nil && ok {
				return true
			}
		}
	}
	return false
}

// SeedOrgPolicies creates the default policies for a new organization and
// assigns the owner role to ownerAccountID.
func (enf *Enforcer) SeedOrgPolicies(orgSlug, ownerAccountID string) error {
	policies := [][]string{
		{"owner", orgSlug, "*", "*"},
		{"admin", orgSlug, "members", "read"},
		{"admin", orgSlug, "members", "write"},
		{"admin", orgSlug, "members", "delete"},
		{"admin", orgSlug, "teams", "*"},
		{"admin", orgSlug, "workspace:*", "*"},
		{"admin", orgSlug, "connection:*", "*"},
	}

	_, err := enf.e.AddPolicies(policies)
	if err != nil {
		return fmt.Errorf("access: seed policies: %w", err)
	}

	_, err = enf.e.AddRoleForUserInDomain(ownerAccountID, "owner", orgSlug)
	if err != nil {
		return fmt.Errorf("access: assign owner role: %w", err)
	}

	return nil
}

// SetOrgRole replaces all roles for accountID in orgSlug with the given role.
func (enf *Enforcer) SetOrgRole(accountID, role, orgSlug string) error {
	_, err := enf.e.DeleteRolesForUserInDomain(accountID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: delete roles: %w", err)
	}

	_, err = enf.e.AddRoleForUserInDomain(accountID, role, orgSlug)
	if err != nil {
		return fmt.Errorf("access: add role: %w", err)
	}

	return nil
}

// RemoveOrgMember removes all roles for accountID in orgSlug.
func (enf *Enforcer) RemoveOrgMember(accountID, orgSlug string) error {
	_, err := enf.e.DeleteRolesForUserInDomain(accountID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: remove member: %w", err)
	}
	return nil
}

// AddTeamMember adds accountID to the given team in orgSlug.
func (enf *Enforcer) AddTeamMember(accountID, teamID, orgSlug string) error {
	_, err := enf.e.AddRoleForUserInDomain(accountID, "team:"+teamID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: add team member: %w", err)
	}
	return nil
}

// RemoveTeamMember removes accountID from the given team in orgSlug.
func (enf *Enforcer) RemoveTeamMember(accountID, teamID, orgSlug string) error {
	_, err := enf.e.DeleteRoleForUserInDomain(accountID, "team:"+teamID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: remove team member: %w", err)
	}
	return nil
}

// GrantWorkspaceAccess grants sub permission to perform act on a workspace.
func (enf *Enforcer) GrantWorkspaceAccess(sub, orgSlug, wsID, act string) error {
	_, err := enf.e.AddPolicy(sub, orgSlug, "workspace:"+wsID, act)
	if err != nil {
		return fmt.Errorf("access: grant workspace access: %w", err)
	}
	return nil
}

// RevokeWorkspaceAccess revokes all of sub's policies on a workspace.
func (enf *Enforcer) RevokeWorkspaceAccess(sub, orgSlug, wsID string) error {
	_, err := enf.e.RemoveFilteredPolicy(0, sub, orgSlug, "workspace:"+wsID)
	if err != nil {
		return fmt.Errorf("access: revoke workspace access: %w", err)
	}
	return nil
}

// ListWorkspaceAccess returns all policy entries for a workspace.
func (enf *Enforcer) ListWorkspaceAccess(orgSlug, wsID string) ([]PolicyEntry, error) {
	policies, err := enf.e.GetFilteredPolicy(1, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("access: list workspace access: %w", err)
	}

	target := "workspace:" + wsID
	var entries []PolicyEntry
	for _, p := range policies {
		if len(p) >= 4 && p[2] == target {
			entries = append(entries, PolicyEntry{Subject: p[0], Action: p[3]})
		}
	}
	return entries, nil
}

// GrantConnectionOverride grants sub a direct override for a connection.
func (enf *Enforcer) GrantConnectionOverride(sub, orgSlug, connID, act string) error {
	_, err := enf.e.AddPolicy(sub, orgSlug, "connection:"+connID, act)
	if err != nil {
		return fmt.Errorf("access: grant connection override: %w", err)
	}
	return nil
}

// RevokeConnectionOverride revokes sub's direct connection override.
func (enf *Enforcer) RevokeConnectionOverride(sub, orgSlug, connID string) error {
	_, err := enf.e.RemoveFilteredPolicy(0, sub, orgSlug, "connection:"+connID)
	if err != nil {
		return fmt.Errorf("access: revoke connection override: %w", err)
	}
	return nil
}

// ListConnectionOverrides returns all policy entries for a connection.
func (enf *Enforcer) ListConnectionOverrides(orgSlug, connID string) ([]PolicyEntry, error) {
	policies, err := enf.e.GetFilteredPolicy(1, orgSlug)
	if err != nil {
		return nil, fmt.Errorf("access: list connection overrides: %w", err)
	}

	target := "connection:" + connID
	var entries []PolicyEntry
	for _, p := range policies {
		if len(p) >= 4 && p[2] == target {
			entries = append(entries, PolicyEntry{Subject: p[0], Action: p[3]})
		}
	}
	return entries, nil
}

// SeedCustomRole creates a custom role with the given actions on a workspace.
func (enf *Enforcer) SeedCustomRole(orgSlug, roleID, wsID string, actions []string) error {
	for _, action := range actions {
		_, err := enf.e.AddPolicy("role:"+roleID, orgSlug, "workspace:"+wsID, action)
		if err != nil {
			return fmt.Errorf("access: seed custom role: %w", err)
		}
	}
	return nil
}

// DeleteCustomRole removes all policies for a custom role.
func (enf *Enforcer) DeleteCustomRole(orgSlug, roleID string) error {
	_, err := enf.e.RemoveFilteredPolicy(0, "role:"+roleID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: delete custom role: %w", err)
	}
	return nil
}

// AssignCustomRole assigns a custom role to a subject in a domain.
func (enf *Enforcer) AssignCustomRole(sub, orgSlug, roleID string) error {
	_, err := enf.e.AddRoleForUserInDomain(sub, "role:"+roleID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: assign custom role: %w", err)
	}
	return nil
}

// RevokeCustomRole revokes a custom role from a subject in a domain.
func (enf *Enforcer) RevokeCustomRole(sub, orgSlug, roleID string) error {
	_, err := enf.e.DeleteRoleForUserInDomain(sub, "role:"+roleID, orgSlug)
	if err != nil {
		return fmt.Errorf("access: revoke custom role: %w", err)
	}
	return nil
}

// ListCustomRoleAssignees returns the subjects assigned to a custom role.
func (enf *Enforcer) ListCustomRoleAssignees(orgSlug, roleID string) ([]string, error) {
	users := enf.e.GetUsersForRoleInDomain("role:"+roleID, orgSlug)
	return users, nil
}
