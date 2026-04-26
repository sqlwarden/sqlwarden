package database

import (
	"fmt"
	"strings"
)

func discoveryPermissionExpr(alias string, prefixes []string) string {
	parts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		parts = append(parts, fmt.Sprintf("%s.permission LIKE '%s%%'", alias, prefix))
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func discoveryRoleBindingExists(bindingAlias, roleAlias, permissionAlias, resourceType, resourceIDExpr, permissionExpr string) string {
	return fmt.Sprintf(`
EXISTS (
    SELECT 1
    FROM role_bindings %s
    JOIN roles %s ON %s.id = %s.role_id
    JOIN role_permissions %s ON %s.role_id = %s.role_id
    WHERE %s.org_id = ?
      AND %s.resource_type = '%s' AND %s.resource_id = %s
      AND %s
      AND (
        (%s.subject_type = 'account' AND %s.subject_id = ?)
        OR (%s.subject_type = 'team' AND %s.subject_id IN (SELECT team_id FROM my_teams))
        OR (%s.subject_type = 'org_members' AND %s.subject_id IN (SELECT org_id FROM my_org_memberships))
      )
)`, bindingAlias, roleAlias, roleAlias, bindingAlias, permissionAlias, permissionAlias, bindingAlias, bindingAlias, bindingAlias, resourceType, bindingAlias, resourceIDExpr, permissionExpr, bindingAlias, bindingAlias, bindingAlias, bindingAlias, bindingAlias, bindingAlias)
}

var (
	workspaceDiscoveryOrgPermissionExpr           = discoveryPermissionExpr("rp", []string{"ws:", "env:", "conn:", "policy:"})
	workspaceDiscoveryWorkspacePermissionExpr     = discoveryPermissionExpr("rp", []string{"ws:", "env:", "conn:", "policy:"})
	workspaceDiscoveryEnvironmentPermissionExpr   = discoveryPermissionExpr("rp", []string{"env:", "conn:"})
	workspaceDiscoveryConnectionPermissionExpr    = discoveryPermissionExpr("rp", []string{"conn:"})
	environmentDiscoveryOrgPermissionExpr         = discoveryPermissionExpr("rp", []string{"env:", "conn:"})
	environmentDiscoveryWorkspacePermissionExpr   = discoveryPermissionExpr("rp", []string{"env:", "conn:"})
	environmentDiscoveryEnvironmentPermissionExpr = discoveryPermissionExpr("rp", []string{"env:", "conn:"})
	environmentDiscoveryConnectionPermissionExpr  = discoveryPermissionExpr("rp", []string{"conn:"})
	connectionDiscoveryPermissionExpr             = discoveryPermissionExpr("rp", []string{"conn:"})
)
