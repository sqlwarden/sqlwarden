package access

import (
	"fmt"
	"sort"
)

// PermissionRegistration adds distribution-owned permission metadata before startup.
type PermissionRegistration struct {
	ID                    string
	Label                 string
	Description           string
	Group                 string
	Scopes                []string
	Resources             []string
	DefaultOrgRoles       []string
	DefaultWorkspaceRoles []string
}

// Registry is an application-local permission catalog. Registration must finish
// before the application starts serving requests.
type Registry struct {
	catalog                []PermissionDefinition
	definitions            map[string]PermissionDefinition
	scopes                 map[string][]string
	resources              map[string][]string
	orgRoles               map[string][]string
	workspaceRoles         map[string][]string
	orgDescriptions        map[string]string
	workspaceDescriptions  map[string]string
	addedOrgDefaults       map[string][]string
	addedWorkspaceDefaults map[string][]string
}

func NewRegistry() *Registry {
	return &Registry{
		catalog:                append([]PermissionDefinition(nil), PermissionCatalog...),
		definitions:            cloneDefinitions(PermissionDefinitions),
		scopes:                 cloneSliceMap(ScopePermissions),
		resources:              cloneSliceMap(ResourcePermissions),
		orgRoles:               cloneSliceMap(OrgBuiltinRoles),
		workspaceRoles:         cloneSliceMap(WorkspaceBuiltinRoles),
		orgDescriptions:        cloneStringMap(OrgBuiltinRoleDescriptions),
		workspaceDescriptions:  cloneStringMap(WorkspaceBuiltinRoleDescriptions),
		addedOrgDefaults:       map[string][]string{},
		addedWorkspaceDefaults: map[string][]string{},
	}
}

func (r *Registry) Add(input PermissionRegistration) error {
	if input.ID == "" {
		return fmt.Errorf("permission ID is required")
	}
	if _, exists := r.definitions[input.ID]; exists {
		return fmt.Errorf("permission %q already exists", input.ID)
	}
	for _, scope := range input.Scopes {
		if _, exists := r.scopes[scope]; !exists {
			return fmt.Errorf("unsupported permission scope %q", scope)
		}
	}
	for _, resource := range input.Resources {
		if _, exists := r.resources[resource]; !exists {
			return fmt.Errorf("unsupported permission resource %q", resource)
		}
	}
	for _, role := range input.DefaultOrgRoles {
		if _, exists := r.orgRoles[role]; !exists {
			return fmt.Errorf("unsupported organization builtin role %q", role)
		}
	}
	for _, role := range input.DefaultWorkspaceRoles {
		if _, exists := r.workspaceRoles[role]; !exists {
			return fmt.Errorf("unsupported workspace builtin role %q", role)
		}
	}
	if len(input.Scopes) == 0 {
		return fmt.Errorf("permission %q must declare at least one scope", input.ID)
	}
	if len(input.Resources) == 0 {
		return fmt.Errorf("permission %q must declare at least one resource", input.ID)
	}
	if len(input.DefaultOrgRoles) > 0 && !contains(input.Scopes, "org") {
		return fmt.Errorf("permission %q must support org scope to be a default organization role permission", input.ID)
	}
	if len(input.DefaultWorkspaceRoles) > 0 && !contains(input.Scopes, "workspace") {
		return fmt.Errorf("permission %q must support workspace scope to be a default workspace role permission", input.ID)
	}
	definition := PermissionDefinition{Key: input.ID, Label: input.Label, Description: input.Description, Group: input.Group}
	r.catalog = append(r.catalog, definition)
	r.definitions[input.ID] = definition
	for _, scope := range input.Scopes {
		r.scopes[scope] = appendUnique(r.scopes[scope], input.ID)
	}
	for _, resource := range input.Resources {
		r.resources[resource] = appendUnique(r.resources[resource], input.ID)
	}
	for _, role := range input.DefaultOrgRoles {
		r.orgRoles[role] = appendUnique(r.orgRoles[role], input.ID)
		r.addedOrgDefaults[role] = appendUnique(r.addedOrgDefaults[role], input.ID)
	}
	for _, role := range input.DefaultWorkspaceRoles {
		r.workspaceRoles[role] = appendUnique(r.workspaceRoles[role], input.ID)
		r.addedWorkspaceDefaults[role] = appendUnique(r.addedWorkspaceDefaults[role], input.ID)
	}
	return nil
}

func (r *Registry) Valid(permission string) bool { _, ok := r.definitions[permission]; return ok }
func (r *Registry) ValidForScope(permission, scope string) bool {
	return contains(r.scopes[scope], permission)
}
func (r *Registry) ValidForResource(permission, resource string) bool {
	return contains(r.resources[resource], permission)
}
func (r *Registry) ResourcePermissions(resource string) []string {
	return append([]string(nil), r.resources[resource]...)
}
func (r *Registry) ScopePermissions(scope string) []string {
	return append([]string(nil), r.scopes[scope]...)
}
func (r *Registry) AllDefinitions() []PermissionDefinition {
	return append([]PermissionDefinition(nil), r.catalog...)
}
func (r *Registry) ScopeMap() map[string][]string       { return cloneSliceMap(r.scopes) }
func (r *Registry) ResourceMap() map[string][]string    { return cloneSliceMap(r.resources) }
func (r *Registry) OrgRoles() map[string][]string       { return cloneSliceMap(r.orgRoles) }
func (r *Registry) WorkspaceRoles() map[string][]string { return cloneSliceMap(r.workspaceRoles) }
func (r *Registry) OrgDescriptions() map[string]string  { return cloneStringMap(r.orgDescriptions) }
func (r *Registry) WorkspaceDescriptions() map[string]string {
	return cloneStringMap(r.workspaceDescriptions)
}
func (r *Registry) AddedOrgDefaults() map[string][]string { return cloneSliceMap(r.addedOrgDefaults) }
func (r *Registry) AddedWorkspaceDefaults() map[string][]string {
	return cloneSliceMap(r.addedWorkspaceDefaults)
}

func (r *Registry) All() []string {
	result := make([]string, 0, len(r.catalog))
	for _, definition := range r.catalog {
		result = append(result, definition.Key)
	}
	return result
}

func (r *Registry) ScopeDefinitions(scope string) []PermissionDefinition {
	return r.definitionsFor(r.scopes[scope])
}
func (r *Registry) ScopeDefinitionMap() map[string][]PermissionDefinition {
	return r.definitionMap(r.scopes)
}
func (r *Registry) ResourceDefinitionMap() map[string][]PermissionDefinition {
	return r.definitionMap(r.resources)
}

func (r *Registry) definitionMap(source map[string][]string) map[string][]PermissionDefinition {
	result := make(map[string][]PermissionDefinition, len(source))
	for key, values := range source {
		result[key] = r.definitionsFor(values)
	}
	return result
}

func (r *Registry) definitionsFor(values []string) []PermissionDefinition {
	result := make([]PermissionDefinition, 0, len(values))
	for _, value := range values {
		if definition, ok := r.definitions[value]; ok {
			result = append(result, definition)
		}
	}
	return result
}

func (r *Registry) AddedPermissionIDs() []string {
	core := make(map[string]struct{}, len(PermissionCatalog))
	for _, definition := range PermissionCatalog {
		core[definition.Key] = struct{}{}
	}
	var result []string
	for permission := range r.definitions {
		if _, ok := core[permission]; !ok {
			result = append(result, permission)
		}
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, value string) []string {
	if contains(values, value) {
		return values
	}
	return append(values, value)
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func cloneSliceMap(source map[string][]string) map[string][]string {
	result := make(map[string][]string, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}
func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
func cloneDefinitions(source map[string]PermissionDefinition) map[string]PermissionDefinition {
	result := make(map[string]PermissionDefinition, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
