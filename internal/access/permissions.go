package access

// Permission constants — single source of truth. Never use raw strings elsewhere.
const (
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

	PermEnvRead   = "env:read"
	PermEnvWrite  = "env:write"
	PermEnvCreate = "env:create"
	PermEnvDelete = "env:delete"
	PermEnvDeploy = "env:deploy"

	PermConnMetadata = "conn:metadata"
	PermConnRead     = "conn:read"
	PermConnWrite    = "conn:write"
	PermConnCreate   = "conn:create"
	PermConnDelete   = "conn:delete"
	PermConnExecute  = "conn:execute"

	PermQueryRead    = "query:read"
	PermQueryWrite   = "query:write"
	PermQueryDelete  = "query:delete"
	PermQueryExecute = "query:execute"
	PermQueryShare   = "query:share"

	PermJobRead    = "job:read"
	PermJobWrite   = "job:write"
	PermJobDelete  = "job:delete"
	PermJobExecute = "job:execute"

	PermFileRead   = "file:read"
	PermFileWrite  = "file:write"
	PermFileDelete = "file:delete"

	PermPolicyRead   = "policy:read"
	PermPolicyModify = "policy:modify"
)

// ScopePermissions maps scope_type to permissions valid for roles of that scope.
var ScopePermissions = map[string][]string{
	"org": {
		PermOrgRead, PermOrgWrite, PermOrgDelete, PermOrgInvite,
		PermOrgAssignRoles, PermOrgTransferOwnership,
		PermWsRead, PermWsWrite, PermWsCreate, PermWsDelete,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
		PermJobRead, PermJobWrite, PermJobDelete, PermJobExecute,
		PermFileRead, PermFileWrite, PermFileDelete,
		PermPolicyRead, PermPolicyModify,
	},
	"workspace": {
		PermWsRead, PermWsWrite,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
		PermJobRead, PermJobWrite, PermJobDelete, PermJobExecute,
		PermFileRead, PermFileWrite, PermFileDelete,
		PermPolicyRead, PermPolicyModify,
	},
	"environment": {
		PermEnvRead, PermEnvWrite, PermEnvDelete, PermEnvDeploy,
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
	},
	"connection": {
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
	},
}

// OrgBuiltinRoles are seeded once per org by SeedOrg.
// owner and admin are bound at the org resource; they gain full access to all workspaces
// via the ancestry traversal (org → workspace → connection).
var OrgBuiltinRoles = map[string][]string{
	"owner": ScopePermissions["org"],
	"admin": {
		PermOrgRead, PermOrgWrite, PermOrgInvite, PermOrgAssignRoles,
		PermWsCreate, PermWsDelete, PermWsRead, PermWsWrite,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
		PermJobRead, PermJobWrite, PermJobDelete, PermJobExecute,
		PermFileRead, PermFileWrite, PermFileDelete,
		PermPolicyRead, PermPolicyModify,
	},
}

// WorkspaceBuiltinRoles are seeded per workspace by SeedWorkspace.
// They are bound at the workspace resource and scoped to that workspace only.
var WorkspaceBuiltinRoles = map[string][]string{
	"ws:admin": {
		PermWsRead, PermWsWrite,
		PermEnvRead, PermEnvWrite, PermEnvCreate, PermEnvDelete, PermEnvDeploy,
		PermConnMetadata, PermConnRead, PermConnWrite, PermConnCreate, PermConnDelete, PermConnExecute,
		PermQueryRead, PermQueryWrite, PermQueryDelete, PermQueryExecute, PermQueryShare,
		PermJobRead, PermJobWrite, PermJobDelete, PermJobExecute,
		PermFileRead, PermFileWrite, PermFileDelete,
		PermPolicyRead, PermPolicyModify,
	},
	"ws:member": {
		PermWsRead,
		PermEnvRead,
		PermConnMetadata, PermConnRead, PermConnExecute,
		PermQueryRead, PermQueryExecute,
		PermJobRead, PermJobExecute,
		PermFileRead,
	},
}

var allPermissionSet map[string]bool

func init() {
	allPermissionSet = make(map[string]bool)
	for _, perms := range ScopePermissions {
		for _, p := range perms {
			allPermissionSet[p] = true
		}
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

// AllPermissions returns all known permission strings (non-deterministic order).
func AllPermissions() []string {
	all := make([]string, 0, len(allPermissionSet))
	for p := range allPermissionSet {
		all = append(all, p)
	}
	return all
}
