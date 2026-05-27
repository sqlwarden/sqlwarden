package access

// Permission constants — single source of truth. Never use raw strings elsewhere.
const (
	SubjectTypeAccount          = "account"
	SubjectTypeTeam             = "team"
	SubjectTypeOrgMembers       = "org_members"
	SubjectTypeWorkspaceMembers = "workspace_members"

	PermOrgRead              = "org:read"
	PermOrgWrite             = "org:write"
	PermOrgDelete            = "org:delete"
	PermOrgInvite            = "org:invite"
	PermOrgAssignRoles       = "org:assign_roles"
	PermOrgTransferOwnership = "org:transfer_ownership"

	PermWsRead   = "ws:read"
	PermWsWrite  = "ws:write"
	PermWsCreate = "ws:create"
	PermWsDelete = "ws:delete"

	PermWsFileRead   = "wsfile:read"
	PermWsFileCreate = "wsfile:create"
	PermWsFileWrite  = "wsfile:write"
	PermWsFileDelete = "wsfile:delete"

	PermEnvRead   = "env:read"
	PermEnvWrite  = "env:write"
	PermEnvCreate = "env:create"
	PermEnvDelete = "env:delete"
	PermEnvDeploy = "env:deploy"

	PermConnRead    = "conn:read"
	PermConnWrite   = "conn:write"
	PermConnCreate  = "conn:create"
	PermConnDelete  = "conn:delete"
	PermConnExecute = "conn:execute"
	PermConnDQL     = "conn:dql"
	PermConnDML     = "conn:dml"
	PermConnDDL     = "conn:ddl"

	PermPolicyRead   = "policy:read"
	PermPolicyModify = "policy:modify"
)

const (
	BuiltinOrgOwnerRole        = "Organization Owner"
	BuiltinOrgAdminRole        = "Organization Admin"
	BuiltinOrgMemberRole       = "Organization Member"
	BuiltinWorkspaceAdminRole  = "Workspace Admin"
	BuiltinWorkspaceMemberRole = "Workspace Member"
)

type PermissionDefinition struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"`
}

var PermissionCatalog = []PermissionDefinition{
	{Key: PermOrgRead, Label: "View organization", Description: "View organization details, members, teams, and other non-sensitive organization metadata.", Group: "Organization"},
	{Key: PermOrgWrite, Label: "Manage organization", Description: "Update organization settings and manage organization-level membership structures.", Group: "Organization"},
	{Key: PermOrgDelete, Label: "Delete organization", Description: "Delete the organization and its owned resources.", Group: "Organization"},
	{Key: PermOrgInvite, Label: "Invite members", Description: "Add existing accounts to the organization and invite new members.", Group: "Organization"},
	{Key: PermOrgAssignRoles, Label: "Assign organization roles", Description: "Change organization member roles and role assignments.", Group: "Organization"},
	{Key: PermOrgTransferOwnership, Label: "Transfer ownership", Description: "Transfer organization ownership to another account.", Group: "Organization"},

	{Key: PermWsRead, Label: "View workspaces", Description: "View workspace details and discover accessible workspace content.", Group: "Workspace"},
	{Key: PermWsWrite, Label: "Manage workspaces", Description: "Update workspace details and workspace-level settings.", Group: "Workspace"},
	{Key: PermWsCreate, Label: "Create workspaces", Description: "Create new workspaces in the organization.", Group: "Workspace"},
	{Key: PermWsDelete, Label: "Delete workspaces", Description: "Delete workspaces and their owned resources.", Group: "Workspace"},
	{Key: PermWsFileRead, Label: "View shared workspace files", Description: "List, open, and download shared files in a workspace.", Group: "Workspace Files"},
	{Key: PermWsFileCreate, Label: "Create shared workspace files", Description: "Create shared files and folders in a workspace.", Group: "Workspace Files"},
	{Key: PermWsFileWrite, Label: "Manage shared workspace files", Description: "Update, rename, and move shared files and folders in a workspace.", Group: "Workspace Files"},
	{Key: PermWsFileDelete, Label: "Delete shared workspace files", Description: "Delete shared files and folders in a workspace.", Group: "Workspace Files"},

	{Key: PermEnvRead, Label: "View environments", Description: "View environment details and discover accessible environment content.", Group: "Environment"},
	{Key: PermEnvWrite, Label: "Manage environments", Description: "Update environment details and environment-level settings.", Group: "Environment"},
	{Key: PermEnvCreate, Label: "Create environments", Description: "Create environments inside accessible workspaces.", Group: "Environment"},
	{Key: PermEnvDelete, Label: "Delete environments", Description: "Delete environments and their owned resources.", Group: "Environment"},
	{Key: PermEnvDeploy, Label: "Deploy to environments", Description: "Run deployment-oriented environment actions.", Group: "Environment"},

	{Key: PermConnRead, Label: "View connections", Description: "View connection metadata without exposing the DSN.", Group: "Connection"},
	{Key: PermConnWrite, Label: "Manage connections", Description: "Update connection configuration, including sensitive DSN changes where allowed.", Group: "Connection"},
	{Key: PermConnCreate, Label: "Create connections", Description: "Create and test new database connections.", Group: "Connection"},
	{Key: PermConnDelete, Label: "Delete connections", Description: "Delete database connections.", Group: "Connection"},
	{Key: PermConnExecute, Label: "Execute all queries", Description: "Run DQL, DML, and DDL queries through accessible connections.", Group: "Connection"},
	{Key: PermConnDQL, Label: "Run read queries", Description: "Run DQL read queries such as SELECT.", Group: "Connection"},
	{Key: PermConnDML, Label: "Run data-change queries", Description: "Run DML queries such as INSERT, UPDATE, and DELETE.", Group: "Connection"},
	{Key: PermConnDDL, Label: "Run schema-change queries", Description: "Run DDL queries such as CREATE, ALTER, and DROP.", Group: "Connection"},

	{Key: PermPolicyRead, Label: "View policies", Description: "View roles, permissions, and policy bindings for the resource scope.", Group: "Policy"},
	{Key: PermPolicyModify, Label: "Manage policies", Description: "Create, update, grant, revoke, and delete roles and policy bindings for the resource scope.", Group: "Policy"},
}

// ScopePermissions maps role scope_type to permissions that roles of that scope
// may contain. This is role-authoring validation: it answers whether a role scoped
// to org, workspace, environment, or connection is allowed to include a permission.
var ScopePermissions = map[string][]string{
	"org": {
		PermOrgRead, PermOrgWrite, PermOrgDelete, PermOrgInvite,
		PermOrgAssignRoles, PermOrgTransferOwnership,
		PermWsRead, PermWsWrite, PermWsCreate, PermWsDelete,
		PermWsFileRead, PermWsFileCreate, PermWsFileWrite, PermWsFileDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
		PermPolicyRead, PermPolicyModify,
	},
	"workspace": {
		PermWsRead, PermWsWrite,
		PermWsFileRead, PermWsFileCreate, PermWsFileWrite, PermWsFileDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
		PermPolicyRead, PermPolicyModify,
	},
	"environment": {
		PermEnvRead, PermEnvWrite, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
	},
	"connection": {
		PermConnRead, PermConnWrite, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
	},
}

// ResourcePermissions maps resource_type to permissions that are meaningful when
// evaluating effective permissions for that resource context. This is capability
// display: it answers whether an inherited permission should be exposed for the
// target resource. It is deliberately separate from ScopePermissions because role
// authoring scope and effective resource applicability are related but not always
// identical.
var ResourcePermissions = map[string][]string{
	"org": ScopePermissions["org"],
	"workspace": {
		PermWsRead, PermWsWrite, PermWsDelete,
		PermWsFileRead, PermWsFileCreate, PermWsFileWrite, PermWsFileDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
		PermPolicyRead, PermPolicyModify,
	},
	"environment": {
		PermEnvRead, PermEnvWrite, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
	},
	"connection": {
		PermConnRead, PermConnWrite, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
	},
}

// OrgBuiltinRoles are seeded once per org by SeedOrg.
// owner and admin are bound at the org resource; they gain full access to all workspaces
// via the ancestry traversal (org → workspace → connection).
var OrgBuiltinRoles = map[string][]string{
	BuiltinOrgOwnerRole: ScopePermissions["org"],
	BuiltinOrgAdminRole: {
		PermOrgRead, PermOrgWrite, PermOrgInvite, PermOrgAssignRoles,
		PermWsCreate, PermWsDelete, PermWsRead, PermWsWrite,
		PermWsFileRead, PermWsFileCreate, PermWsFileWrite, PermWsFileDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
		PermPolicyRead, PermPolicyModify,
	},
	BuiltinOrgMemberRole: {
		PermOrgRead,
	},
}

var OrgBuiltinRoleDescriptions = map[string]string{
	BuiltinOrgOwnerRole:  "Full organization owner access, including all organization, workspace, environment, connection, and policy permissions.",
	BuiltinOrgAdminRole:  "Administrative organization access for day-to-day management without ownership transfer or organization deletion.",
	BuiltinOrgMemberRole: "Baseline organization membership with read access to organization-level information.",
}

// WorkspaceBuiltinRoles are seeded per workspace by SeedWorkspace.
// They are bound at the workspace resource and scoped to that workspace only.
var WorkspaceBuiltinRoles = map[string][]string{
	BuiltinWorkspaceAdminRole: {
		PermWsRead, PermWsWrite,
		PermWsFileRead, PermWsFileCreate, PermWsFileWrite, PermWsFileDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermConnDQL, PermConnDML, PermConnDDL,
		PermPolicyRead, PermPolicyModify,
	},
	BuiltinWorkspaceMemberRole: {
		PermWsRead,
	},
}

var WorkspaceBuiltinRoleDescriptions = map[string]string{
	BuiltinWorkspaceAdminRole:  "Full workspace administration access, including shared files, workspace updates, environments, connections, queries, and workspace policies.",
	BuiltinWorkspaceMemberRole: "Baseline workspace member access with workspace visibility.",
}

var (
	allPermissionSet      map[string]bool
	PermissionDefinitions map[string]PermissionDefinition
)

func init() {
	allPermissionSet = make(map[string]bool)
	PermissionDefinitions = make(map[string]PermissionDefinition, len(PermissionCatalog))
	for _, definition := range PermissionCatalog {
		allPermissionSet[definition.Key] = true
		PermissionDefinitions[definition.Key] = definition
	}
}

// ValidPermission returns true if p is a known permission string.
func ValidPermission(p string) bool { return allPermissionSet[p] }

// ValidForScope returns true if permission p is valid for roles of scopeType.
func ValidForScope(p, scopeType string) bool {
	for _, sp := range ScopePermissions[scopeType] {
		if sp == p {
			return true
		}
	}
	return false
}

// ValidForResource returns true if permission p is meaningful for an effective
// permissions response targeting resourceType.
func ValidForResource(p, resourceType string) bool {
	for _, rp := range ResourcePermissions[resourceType] {
		if rp == p {
			return true
		}
	}
	return false
}

// AllPermissions returns all known permission strings in catalog order.
func AllPermissions() []string {
	all := make([]string, 0, len(PermissionCatalog))
	for _, definition := range PermissionCatalog {
		all = append(all, definition.Key)
	}
	return all
}

func AllPermissionDefinitions() []PermissionDefinition {
	definitions := make([]PermissionDefinition, len(PermissionCatalog))
	copy(definitions, PermissionCatalog)
	return definitions
}

func ScopePermissionDefinitions(scopeType string) []PermissionDefinition {
	permissions := ScopePermissions[scopeType]
	definitions := make([]PermissionDefinition, 0, len(permissions))
	for _, permission := range permissions {
		if definition, ok := PermissionDefinitions[permission]; ok {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func ScopePermissionDefinitionMap() map[string][]PermissionDefinition {
	result := make(map[string][]PermissionDefinition, len(ScopePermissions))
	for scopeType := range ScopePermissions {
		result[scopeType] = ScopePermissionDefinitions(scopeType)
	}
	return result
}

func ResourcePermissionDefinitions(resourceType string) []PermissionDefinition {
	permissions := ResourcePermissions[resourceType]
	definitions := make([]PermissionDefinition, 0, len(permissions))
	for _, permission := range permissions {
		if definition, ok := PermissionDefinitions[permission]; ok {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func ResourcePermissionDefinitionMap() map[string][]PermissionDefinition {
	result := make(map[string][]PermissionDefinition, len(ResourcePermissions))
	for resourceType := range ResourcePermissions {
		result[resourceType] = ResourcePermissionDefinitions(resourceType)
	}
	return result
}
