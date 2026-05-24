package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/uptrace/bun"
)

// Enforcer evaluates permissions using domain tables (no Casbin).
type Enforcer struct {
	db    *bun.DB
	cache Cache
}

// New creates an Enforcer backed by the given database.
func New(db *bun.DB) (*Enforcer, error) {
	return &Enforcer{db: db, cache: NewMemoryCache()}, nil
}

// Can returns true if accountID holds permission on the given resource within orgID.
// ownerType="space" short-circuits to true — users own their space entirely.
func (e *Enforcer) Can(ctx context.Context,
	accountID, orgID int64,
	ownerType, resourceType string, resourceID int64,
	permission string,
) bool {
	if ownerType == "space" {
		return true
	}

	principals, err := e.principalsFor(ctx, orgID, accountID)
	if err != nil {
		return false
	}

	ancestors, err := e.ancestryFor(ctx, ownerType, resourceType, resourceID, orgID)
	if err != nil {
		return false
	}

	policy, err := e.orgPolicy(ctx, orgID)
	if err != nil {
		return false
	}

	return e.checkPolicy(policy, accountID, principals, ancestors, permission)
}

// EffectivePermissions returns the sorted set of permissions accountID has for the
// target resource. It evaluates all bindings on the target and its ancestors once,
// then filters permissions to those applicable to the target resource type.
func (e *Enforcer) EffectivePermissions(ctx context.Context,
	accountID, orgID int64,
	ownerType, resourceType string, resourceID int64,
) ([]string, error) {
	if ownerType == "space" {
		permissions := append([]string(nil), ResourcePermissions[resourceType]...)
		sort.Strings(permissions)
		return permissions, nil
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

	permissions := e.effectivePolicyPermissions(policy, accountID, principals, ancestors, resourceType)
	sort.Strings(permissions)
	return permissions, nil
}

// principalsFor returns all principals the account matches within orgID.
func (e *Enforcer) principalsFor(ctx context.Context, orgID, accountID int64) (Principals, error) {
	if principals, ok := e.cache.GetPrincipals(orgID, accountID); ok {
		return principals, nil
	}

	var rows []struct{ TeamID int64 }
	err := e.db.NewSelect().
		TableExpr("team_members tm").
		ColumnExpr("tm.team_id").
		Join("JOIN teams t ON t.id = tm.team_id").
		Where("tm.account_id = ? AND t.org_id = ?", accountID, orgID).
		Scan(ctx, &rows)
	if err != nil {
		return Principals{}, err
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.TeamID
	}

	var workspaceRows []struct{ WorkspaceID int64 }
	err = e.db.NewRaw(`
SELECT DISTINCT workspace_id
FROM (
    SELECT wm.workspace_id
    FROM workspace_members wm
    JOIN workspaces w ON w.id = wm.workspace_id
    JOIN org_members om ON om.org_id = w.owner_id AND om.account_id = wm.account_id
    WHERE wm.account_id = ? AND w.owner_type = 'org' AND w.owner_id = ?
  UNION
    SELECT wt.workspace_id
    FROM workspace_teams wt
    JOIN team_members tm ON tm.team_id = wt.team_id
    JOIN workspaces w ON w.id = wt.workspace_id
    JOIN org_members om ON om.org_id = w.owner_id AND om.account_id = tm.account_id
    WHERE tm.account_id = ? AND w.owner_type = 'org' AND w.owner_id = ?
) workspace_principals`,
		accountID, orgID, accountID, orgID,
	).Scan(ctx, &workspaceRows)
	if err != nil {
		return Principals{}, err
	}

	workspaceIDs := make([]int64, len(workspaceRows))
	for i, r := range workspaceRows {
		workspaceIDs[i] = r.WorkspaceID
	}

	orgMember, err := e.db.NewSelect().
		TableExpr("org_members").
		Where("org_id = ? AND account_id = ?", orgID, accountID).
		Exists(ctx)
	if err != nil {
		return Principals{}, err
	}

	principals := Principals{OrgID: orgID, TeamIDs: ids, WorkspaceMemberIDs: workspaceIDs, OrgMember: orgMember}
	e.cache.SetPrincipals(orgID, accountID, principals)
	return principals, nil
}

// ancestryFor returns all ancestor resource levels for the target resource,
// including the resource itself and the org.
func (e *Enforcer) ancestryFor(ctx context.Context, _ /*ownerType*/ string, resourceType string, resourceID, orgID int64) ([]AncestorLevel, error) {
	// The resource itself.
	levels := []AncestorLevel{{ResourceType: resourceType, ResourceID: resourceID}}

	// For org-level resources, no hierarchy lookup needed.
	if resourceType == "org" {
		return levels, nil
	}

	if cached, ok := e.cache.GetAncestry(resourceType, resourceID); ok {
		return append(levels, cached...), nil
	}

	var rows []struct {
		ParentType string
		ParentID   int64
	}
	err := e.db.NewRaw(`
WITH RECURSIVE ancestors(parent_type, parent_id) AS (
    SELECT parent_type, parent_id
    FROM resource_hierarchy
    WHERE child_type = ? AND child_id = ?
  UNION
    SELECT rh.parent_type, rh.parent_id
    FROM resource_hierarchy rh
    JOIN ancestors a
      ON rh.child_type = a.parent_type
     AND rh.child_id = a.parent_id
)
SELECT DISTINCT parent_type, parent_id
FROM ancestors`,
		resourceType, resourceID,
	).Scan(ctx, &rows)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	ancestry := make([]AncestorLevel, len(rows))
	for i, r := range rows {
		ancestry[i] = AncestorLevel{ResourceType: r.ParentType, ResourceID: r.ParentID}
	}

	// Always include the org itself at the end of the ancestry chain.
	// The "space" short-circuit in Can() means ownerType=="space" never reaches here,
	// so it is always safe to append the org level for permission inheritance.
	hasOrg := false
	for _, a := range ancestry {
		if a.ResourceType == "org" {
			hasOrg = true
			break
		}
	}
	if !hasOrg && orgID != 0 {
		ancestry = append(ancestry, AncestorLevel{ResourceType: "org", ResourceID: orgID})
	}

	e.cache.SetAncestry(resourceType, resourceID, ancestry)
	return append(levels, ancestry...), nil
}

// orgPolicy loads (or returns cached) the full org policy.
func (e *Enforcer) orgPolicy(ctx context.Context, orgID int64) (*OrgPolicy, error) {
	if p, ok := e.cache.GetOrgPolicy(orgID); ok {
		return p, nil
	}

	policy := &OrgPolicy{
		rolePermissions: make(map[int64]map[string]bool),
		roleScopeTypes:  make(map[int64]string),
		roleBindings:    make(map[resourceKey][]cachedRoleBinding),
	}

	// Load role permissions.
	var rpRows []struct {
		RoleID     int64
		ScopeType  string
		Permission string
	}
	err := e.db.NewSelect().
		TableExpr("role_permissions rp").
		ColumnExpr("rp.role_id, r.scope_type, rp.permission").
		Join("JOIN roles r ON r.id = rp.role_id").
		Where("r.org_id = ?", orgID).
		Scan(ctx, &rpRows)
	if err != nil {
		return nil, fmt.Errorf("load role permissions: %w", err)
	}
	for _, r := range rpRows {
		if policy.rolePermissions[r.RoleID] == nil {
			policy.rolePermissions[r.RoleID] = make(map[string]bool)
		}
		policy.roleScopeTypes[r.RoleID] = r.ScopeType
		policy.rolePermissions[r.RoleID][r.Permission] = true
	}

	// Load role bindings.
	var rbRows []struct {
		RoleID       int64
		SubjectType  string
		SubjectID    int64
		ResourceType string
		ResourceID   int64
	}
	err = e.db.NewSelect().
		TableExpr("role_bindings").
		ColumnExpr("role_id, subject_type, subject_id, resource_type, resource_id").
		Where("org_id = ?", orgID).
		Scan(ctx, &rbRows)
	if err != nil {
		return nil, fmt.Errorf("load role bindings: %w", err)
	}
	for _, r := range rbRows {
		key := resourceKey{r.ResourceType, r.ResourceID}
		policy.roleBindings[key] = append(policy.roleBindings[key], cachedRoleBinding{
			roleID:      r.RoleID,
			subjectType: r.SubjectType,
			subjectID:   r.SubjectID,
		})
	}

	e.cache.SetOrgPolicy(orgID, policy)
	return policy, nil
}

// checkPolicy returns true if any cached binding grants permission to the principals at any ancestor level.
func (e *Enforcer) checkPolicy(policy *OrgPolicy, accountID int64, principals Principals, ancestors []AncestorLevel, permission string) bool {
	for _, level := range ancestors {
		key := resourceKey{level.ResourceType, level.ResourceID}

		// Check role bindings at this level.
		for _, rb := range policy.roleBindings[key] {
			if !matchesPrincipal(rb.subjectType, rb.subjectID, accountID, principals) {
				continue
			}
			if perms := policy.rolePermissions[rb.roleID]; perms[permission] {
				return true
			}
		}

	}
	return false
}

func (e *Enforcer) effectivePolicyPermissions(policy *OrgPolicy, accountID int64, principals Principals, ancestors []AncestorLevel, targetResourceType string) []string {
	seen := make(map[string]bool)

	for _, level := range ancestors {
		key := resourceKey{level.ResourceType, level.ResourceID}
		for _, rb := range policy.roleBindings[key] {
			if !matchesPrincipal(rb.subjectType, rb.subjectID, accountID, principals) {
				continue
			}
			roleScopeType := policy.roleScopeTypes[rb.roleID]
			for permission := range policy.rolePermissions[rb.roleID] {
				if !ValidForScope(permission, roleScopeType) {
					continue
				}
				if !ValidForResource(permission, targetResourceType) {
					continue
				}
				seen[permission] = true
			}
		}
	}

	permissions := make([]string, 0, len(seen))
	for permission := range seen {
		permissions = append(permissions, permission)
	}
	return permissions
}

func matchesPrincipal(subjectType string, subjectID, accountID int64, principals Principals) bool {
	if subjectType == SubjectTypeAccount {
		return subjectID == accountID
	}
	if subjectType == SubjectTypeTeam {
		for _, tid := range principals.TeamIDs {
			if tid == subjectID {
				return true
			}
		}
	}
	if subjectType == SubjectTypeOrgMembers {
		return principals.OrgMember && subjectID == principals.OrgID
	}
	if subjectType == SubjectTypeWorkspaceMembers {
		for _, workspaceID := range principals.WorkspaceMemberIDs {
			if workspaceID == subjectID {
				return true
			}
		}
	}
	return false
}

// SeedOrg creates the owner and admin builtin roles for a new org and binds ownerAccountID to
// the owner role. Call this once when creating a new organization.
func (e *Enforcer) SeedOrg(ctx context.Context, orgID, ownerAccountID int64) error {
	for roleName, permissions := range OrgBuiltinRoles {
		roleID, err := e.insertRole(ctx, orgID, nil, roleName, OrgBuiltinRoleDescriptions[roleName], "org", true)
		if err != nil {
			return fmt.Errorf("seed role %s: %w", roleName, err)
		}

		for _, perm := range permissions {
			m := map[string]interface{}{"role_id": roleID, "permission": perm}
			_, err = e.db.NewInsert().
				TableExpr("role_permissions").
				Model(&m).
				Ignore().
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("seed permission %s: %w", perm, err)
			}
		}

		if roleName == BuiltinOrgOwnerRole {
			err = e.bindRoleByID(ctx, orgID, roleID, SubjectTypeAccount, ownerAccountID, "org", orgID, ownerAccountID)
			if err != nil {
				return fmt.Errorf("bind owner role: %w", err)
			}
		}
		if roleName == BuiltinOrgMemberRole {
			err = e.bindRoleByID(ctx, orgID, roleID, SubjectTypeOrgMembers, orgID, "org", orgID, ownerAccountID)
			if err != nil {
				return fmt.Errorf("bind org member role: %w", err)
			}
		}
	}

	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// SeedWorkspace creates the workspace builtin roles for a new workspace and binds
// creatorAccountID to BuiltinWorkspaceAdminRole. Call this once when creating a new workspace.
func (e *Enforcer) SeedWorkspace(ctx context.Context, orgID, workspaceID, creatorAccountID int64) error {
	for roleName, permissions := range WorkspaceBuiltinRoles {
		roleID, err := e.insertRole(ctx, orgID, &workspaceID, roleName, WorkspaceBuiltinRoleDescriptions[roleName], "workspace", true)
		if err != nil {
			return fmt.Errorf("seed workspace role %s: %w", roleName, err)
		}

		for _, perm := range permissions {
			m := map[string]interface{}{"role_id": roleID, "permission": perm}
			_, err = e.db.NewInsert().
				TableExpr("role_permissions").
				Model(&m).
				Ignore().
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("seed workspace permission %s: %w", perm, err)
			}
		}

		if roleName == BuiltinWorkspaceAdminRole {
			err = e.bindRoleByID(ctx, orgID, roleID, SubjectTypeAccount, creatorAccountID, "workspace", workspaceID, creatorAccountID)
			if err != nil {
				return fmt.Errorf("bind %s role: %w", BuiltinWorkspaceAdminRole, err)
			}
		}
		if roleName == BuiltinWorkspaceMemberRole {
			err = e.bindRoleByID(ctx, orgID, roleID, SubjectTypeWorkspaceMembers, workspaceID, "workspace", workspaceID, creatorAccountID)
			if err != nil {
				return fmt.Errorf("bind workspace member role: %w", err)
			}
		}
	}

	wm := map[string]interface{}{
		"workspace_id": workspaceID,
		"account_id":   creatorAccountID,
		"created_by":   creatorAccountID,
	}
	_, err := e.db.NewInsert().
		TableExpr("workspace_members").
		Model(&wm).
		Ignore().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("bind creator workspace membership: %w", err)
	}

	e.cache.InvalidateOrgPolicy(orgID)
	e.cache.InvalidatePrincipals(orgID, creatorAccountID)
	return nil
}

// CreateRole creates a custom (non-builtin) role. Pass workspaceID=nil for an org-level role or
// a non-nil pointer for a workspace-scoped custom role. Returns the new role ID.
func (e *Enforcer) CreateRole(ctx context.Context, orgID int64, workspaceID *int64, name, description, scopeType string, permissions []string) (int64, error) {
	for _, p := range permissions {
		if !ValidForScope(p, scopeType) {
			return 0, fmt.Errorf("%w: permission %q is not valid for scope %q", ErrInvalidScopePermission, p, scopeType)
		}
	}

	roleID, err := e.insertRole(ctx, orgID, workspaceID, name, description, scopeType, false)
	if err != nil {
		return 0, err
	}

	for _, perm := range permissions {
		pm := map[string]interface{}{"role_id": roleID, "permission": perm}
		_, err = e.db.NewInsert().
			TableExpr("role_permissions").
			Model(&pm).
			Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("insert permission: %w", err)
		}
	}

	e.cache.InvalidateOrgPolicy(orgID)
	return roleID, nil
}

// DeleteRole deletes a custom role. Returns an error if the role is builtin.
func (e *Enforcer) DeleteRole(ctx context.Context, roleID, orgID int64) error {
	var isBuiltin bool
	err := e.db.NewSelect().
		TableExpr("roles").
		ColumnExpr("is_builtin").
		Where("id = ? AND org_id = ?", roleID, orgID).
		Scan(ctx, &isBuiltin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRoleNotFound
		}
		return err
	}
	if isBuiltin {
		return ErrBuiltinRole
	}

	_, err = e.db.NewDelete().TableExpr("roles").Where("id = ?", roleID).Exec(ctx)
	if err != nil {
		return err
	}

	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// BindRole assigns a role to a subject at a specific resource.
func (e *Enforcer) BindRole(ctx context.Context, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	err := e.bindRoleByID(ctx, orgID, roleID, subjectType, subjectID, resourceType, resourceID, grantedBy)
	if err != nil {
		return err
	}
	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// UnbindRole removes a role binding by binding ID.
func (e *Enforcer) UnbindRole(ctx context.Context, bindingID, orgID int64) error {
	_, err := e.db.NewDelete().TableExpr("role_bindings").Where("id = ? AND org_id = ?", bindingID, orgID).Exec(ctx)
	if err != nil {
		return err
	}
	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// insertRole inserts a role row and returns its ID. workspaceID=nil creates an org-level role;
// non-nil creates a workspace-scoped role. Idempotent: returns existing ID on conflict.
func (e *Enforcer) insertRole(ctx context.Context, orgID int64, workspaceID *int64, name, description, scopeType string, isBuiltin bool) (int64, error) {
	rm := map[string]interface{}{
		"org_id":       orgID,
		"workspace_id": workspaceID,
		"name":         name,
		"description":  description,
		"scope_type":   scopeType,
		"is_builtin":   isBuiltin,
	}
	var id int64
	err := e.db.NewInsert().
		TableExpr("roles").
		Model(&rm).
		On("CONFLICT DO NOTHING").
		Returning("id").
		Scan(ctx, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	// id=0 means the row already existed (conflict, nothing inserted). Look it up.
	if id == 0 {
		q := e.db.NewSelect().TableExpr("roles").ColumnExpr("id").
			Where("org_id = ? AND name = ?", orgID, name)
		if workspaceID == nil {
			q = q.Where("workspace_id IS NULL")
		} else {
			q = q.Where("workspace_id = ?", *workspaceID)
		}
		if err = q.Scan(ctx, &id); err != nil {
			return 0, err
		}
		if isBuiltin && description != "" {
			_, err = e.db.NewUpdate().
				TableExpr("roles").
				Set("description = ?", description).
				Where("id = ?", id).
				Exec(ctx)
			if err != nil {
				return 0, err
			}
		}
	}
	return id, nil
}

// bindRoleByID inserts a role_bindings row.
func (e *Enforcer) bindRoleByID(ctx context.Context, orgID, roleID int64, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	rbm := map[string]interface{}{
		"org_id":        orgID,
		"role_id":       roleID,
		"subject_type":  subjectType,
		"subject_id":    subjectID,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"created_by":    grantedBy,
	}
	_, err := e.db.NewInsert().
		TableExpr("role_bindings").
		Model(&rbm).
		Ignore().
		Exec(ctx)
	return err
}

// InvalidatePrincipals invalidates the principal cache for an account.
func (e *Enforcer) InvalidatePrincipals(orgID, accountID int64) {
	e.cache.InvalidatePrincipals(orgID, accountID)
}

// InvalidateOrgPolicy invalidates the policy cache for an org.
func (e *Enforcer) InvalidateOrgPolicy(orgID int64) {
	e.cache.InvalidateOrgPolicy(orgID)
}

// InvalidateAncestry invalidates the ancestry cache for a resource.
func (e *Enforcer) InvalidateAncestry(resourceType string, resourceID int64) {
	e.cache.InvalidateAncestry(resourceType, resourceID)
}
