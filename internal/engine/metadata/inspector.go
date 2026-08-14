package metadata

import "context"

// SchemaInspector is the metadata-domain interface a driver implements to report
// its objects in two tiers: a cheap Directory listing, and on-demand detail for
// specific objects. It is satisfied implicitly (Go structural typing); drivers
// need not import this interface, only the metadata types they return.
type SchemaInspector interface {
	// SchemaSpec is pure/static and must not touch the target database.
	SchemaSpec() SchemaSpec
	// InspectDirectory lists objects (names + kinds) without columns/keys.
	InspectDirectory(ctx context.Context, opts DirectoryOptions) (*Directory, error)
	// InspectObjects returns detail only for the requested refs, pushing the
	// ref filter into the underlying query (never fetch-all-then-filter). Refs
	// that do not exist are simply omitted from the result (partial success).
	InspectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

// RelationshipInspector is the OPTIONAL capability to report a scope's
// foreign-key edges cheaply (no column detail). Relational engines implement it;
// engines without a foreign-key concept do not, and callers report 501 via the
// same type-assertion path used for SchemaInspector.
type RelationshipInspector interface {
	InspectRelationshipsInScope(ctx context.Context, scope ScopePath) (*RelationshipGraph, error)
}

// DirectoryOptions optionally limits inspection to one hierarchy root.
type DirectoryOptions struct {
	Root ScopePath
}

// ScopeDiscoverer is the optional cheap connection-time hierarchy capability.
// It lists scopes only and must not inspect database objects.
type ScopeDiscoverer interface {
	DiscoverScopes(ctx context.Context, request ScopeDiscoveryRequest) (*ScopeDiscovery, error)
}

// ScopeDiscoveryRequest identifies the parent whose immediate scopes are needed.
type ScopeDiscoveryRequest struct {
	Parent ScopePath `json:"parent,omitempty"`
}

// ScopeDiscovery reports the current scope and its selectable descendants.
type ScopeDiscovery struct {
	Current ScopePath   `json:"current,omitempty"`
	Scopes  []ScopePath `json:"scopes"`
}
