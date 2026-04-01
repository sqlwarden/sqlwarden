package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

	teamIDs, err := e.principalsFor(ctx, orgID, accountID)
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

	return e.checkPolicy(policy, accountID, teamIDs, ancestors, permission)
}

// principalsFor returns the team IDs the account belongs to within orgID.
func (e *Enforcer) principalsFor(ctx context.Context, orgID, accountID int64) ([]int64, error) {
	if ids, ok := e.cache.GetPrincipals(orgID, accountID); ok {
		return ids, nil
	}

	var rows []struct{ TeamID int64 }
	err := e.db.NewSelect().
		TableExpr("team_members tm").
		ColumnExpr("tm.team_id").
		Join("JOIN teams t ON t.id = tm.team_id").
		Where("tm.account_id = ? AND t.org_id = ?", accountID, orgID).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.TeamID
	}
	e.cache.SetPrincipals(orgID, accountID, ids)
	return ids, nil
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
	err := e.db.NewSelect().
		TableExpr("resource_hierarchy").
		ColumnExpr("parent_type, parent_id").
		Where("child_type = ? AND child_id = ?", resourceType, resourceID).
		Scan(ctx, &rows)
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
		roleBindings:    make(map[resourceKey][]cachedRoleBinding),
		permBindings:    make(map[resourceKey][]cachedPermBinding),
	}

	// Load role permissions.
	var rpRows []struct {
		RoleID     int64
		Permission string
	}
	err := e.db.NewSelect().
		TableExpr("role_permissions rp").
		ColumnExpr("rp.role_id, rp.permission").
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

	// Load permission bindings.
	var pbRows []struct {
		Permission   string
		SubjectType  string
		SubjectID    int64
		ResourceType string
		ResourceID   int64
	}
	err = e.db.NewSelect().
		TableExpr("permission_bindings").
		ColumnExpr("permission, subject_type, subject_id, resource_type, resource_id").
		Where("org_id = ?", orgID).
		Scan(ctx, &pbRows)
	if err != nil {
		return nil, fmt.Errorf("load permission bindings: %w", err)
	}
	for _, r := range pbRows {
		key := resourceKey{r.ResourceType, r.ResourceID}
		policy.permBindings[key] = append(policy.permBindings[key], cachedPermBinding{
			permission:  r.Permission,
			subjectType: r.SubjectType,
			subjectID:   r.SubjectID,
		})
	}

	e.cache.SetOrgPolicy(orgID, policy)
	return policy, nil
}

// checkPolicy returns true if any cached binding grants permission to the principals at any ancestor level.
func (e *Enforcer) checkPolicy(policy *OrgPolicy, accountID int64, teamIDs []int64, ancestors []AncestorLevel, permission string) bool {
	for _, level := range ancestors {
		key := resourceKey{level.ResourceType, level.ResourceID}

		// Check role bindings at this level.
		for _, rb := range policy.roleBindings[key] {
			if !matchesPrincipal(rb.subjectType, rb.subjectID, accountID, teamIDs) {
				continue
			}
			if perms := policy.rolePermissions[rb.roleID]; perms[permission] {
				return true
			}
		}

		// Check direct permission bindings at this level.
		for _, pb := range policy.permBindings[key] {
			if pb.permission == permission && matchesPrincipal(pb.subjectType, pb.subjectID, accountID, teamIDs) {
				return true
			}
		}
	}
	return false
}

func matchesPrincipal(subjectType string, subjectID, accountID int64, teamIDs []int64) bool {
	if subjectType == "account" {
		return subjectID == accountID
	}
	if subjectType == "team" {
		for _, tid := range teamIDs {
			if tid == subjectID {
				return true
			}
		}
	}
	return false
}

// SeedOrg creates the three builtin roles for a new org and binds ownerAccountID to the owner role.
// Call this once when creating a new organization.
func (e *Enforcer) SeedOrg(ctx context.Context, orgID, ownerAccountID int64) error {
	for roleName, permissions := range BuiltinRoles {
		roleID, err := e.insertRole(ctx, orgID, roleName, "", "org", true)
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

		if roleName == "owner" {
			err = e.bindRoleByID(ctx, orgID, roleID, "account", ownerAccountID, "org", orgID, ownerAccountID)
			if err != nil {
				return fmt.Errorf("bind owner role: %w", err)
			}
		}
	}

	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// CreateRole creates a custom (non-builtin) role for an org. Returns the new role ID.
func (e *Enforcer) CreateRole(ctx context.Context, orgID int64, name, description, scopeType string, permissions []string) (int64, error) {
	for _, p := range permissions {
		if !ValidForScope(p, scopeType) {
			return 0, fmt.Errorf("permission %q is not valid for scope %q", p, scopeType)
		}
	}

	roleID, err := e.insertRole(ctx, orgID, name, description, scopeType, false)
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
		return err
	}
	if isBuiltin {
		return errors.New("cannot delete a builtin role")
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

// GrantPermission grants a direct permission binding to a subject at a resource.
func (e *Enforcer) GrantPermission(ctx context.Context, orgID int64, permission, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	if !ValidPermission(permission) {
		return fmt.Errorf("unknown permission: %s", permission)
	}

	pbm := map[string]interface{}{
		"org_id":        orgID,
		"permission":    permission,
		"subject_type":  subjectType,
		"subject_id":    subjectID,
		"resource_type": resourceType,
		"resource_id":   resourceID,
		"created_by":    grantedBy,
	}
	_, err := e.db.NewInsert().
		TableExpr("permission_bindings").
		Model(&pbm).
		Exec(ctx)
	if err != nil {
		return err
	}

	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// GrantPermissions grants multiple direct permission bindings to a subject at a resource in one call.
// All permissions are validated before any insert is attempted.
func (e *Enforcer) GrantPermissions(ctx context.Context, orgID int64, permissions []string, subjectType string, subjectID int64, resourceType string, resourceID int64, grantedBy int64) error {
	for _, p := range permissions {
		if !ValidPermission(p) {
			return fmt.Errorf("unknown permission: %s", p)
		}
	}
	for _, p := range permissions {
		pbm := map[string]interface{}{
			"org_id":        orgID,
			"permission":    p,
			"subject_type":  subjectType,
			"subject_id":    subjectID,
			"resource_type": resourceType,
			"resource_id":   resourceID,
			"created_by":    grantedBy,
		}
		_, err := e.db.NewInsert().
			TableExpr("permission_bindings").
			Model(&pbm).
			Ignore().
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("grant permission %s: %w", p, err)
		}
	}
	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// RevokePermission removes a permission binding by binding ID.
func (e *Enforcer) RevokePermission(ctx context.Context, bindingID, orgID int64) error {
	_, err := e.db.NewDelete().TableExpr("permission_bindings").Where("id = ? AND org_id = ?", bindingID, orgID).Exec(ctx)
	if err != nil {
		return err
	}
	e.cache.InvalidateOrgPolicy(orgID)
	return nil
}

// insertRole inserts a role row and returns its ID.
func (e *Enforcer) insertRole(ctx context.Context, orgID int64, name, description, scopeType string, isBuiltin bool) (int64, error) {
	var id int64
	rm := map[string]interface{}{
		"org_id":      orgID,
		"name":        name,
		"description": description,
		"scope_type":  scopeType,
		"is_builtin":  isBuiltin,
	}
	err := e.db.NewInsert().
		TableExpr("roles").
		Model(&rm).
		On("CONFLICT (org_id, name) DO UPDATE SET updated_at = NOW()").
		Returning("id").
		Scan(ctx, &id)
	return id, err
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

// InvalidateAncestry invalidates the ancestry cache for a resource.
func (e *Enforcer) InvalidateAncestry(resourceType string, resourceID int64) {
	e.cache.InvalidateAncestry(resourceType, resourceID)
}
