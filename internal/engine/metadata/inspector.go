package metadata

import "context"

// DefinitionInspector is the OPTIONAL capability to fetch a single object's
// canonical text definition (a table/view DDL, a routine body) on demand. Engines
// whose bulk InspectObjects already embeds a "DDL"/"Definition" source descriptor
// do not need it; engines that omit the definition from bulk inspection because
// producing it per object is expensive expose it here for lazy retrieval. Callers
// report 501 via the same type-assertion path used for SchemaInspector.
type DefinitionInspector interface {
	// InspectDefinition returns a single "source" descriptor for the object, or
	// nil when no definition is available (unsupported kind, insufficient
	// privilege). A nil error with a nil descriptor is a valid "not available".
	InspectDefinition(ctx context.Context, ref ObjectRef) (*Descriptor, error)
}

// RelationshipInspector is the OPTIONAL capability to report a scope's
// foreign-key edges cheaply (no column detail). Relational engines implement it;
// engines without a foreign-key concept do not, and callers report 501 via the
// same type-assertion path used for SchemaInspector.
type RelationshipInspector interface {
	InspectRelationshipsInScope(ctx context.Context, scope ScopePath) (*RelationshipGraph, error)
}

// SchemaInspector is the navigator capability: a static grammar whose folder
// loaders run against a database-scoped Querier, plus on-demand object detail.
type SchemaInspector interface {
	// Tree is static and must not touch the target database.
	Tree() Tree
	// Querier returns a handle scoped to database ("" = connection default).
	Querier(ctx context.Context, database string) (Querier, error)
	// InspectObjects returns detail only for the requested refs.
	InspectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

type ObjectInspector interface {
	InspectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

// ScopeDiscovery reports the current scope and its selectable descendants.
type ScopeDiscovery struct {
	Current ScopePath   `json:"current,omitempty"`
	Scopes  []ScopePath `json:"scopes"`
}
