package schema

import "context"

// SchemaInspector is the schema-domain interface a driver implements to report
// its objects in two tiers: a cheap Catalog listing, and on-demand detail for
// specific objects. It is satisfied implicitly (Go structural typing); drivers
// need not import this interface, only the schema types they return.
type SchemaInspector interface {
	// SchemaSpec is pure/static and must not touch the target database.
	SchemaSpec() SchemaSpec
	// InspectCatalog lists objects (names + kinds) without columns/keys.
	InspectCatalog(ctx context.Context, opts CatalogOptions) (*Catalog, error)
	// InspectObjects returns detail only for the requested refs, pushing the
	// ref filter into the underlying query (never fetch-all-then-filter). Refs
	// that do not exist are simply omitted from the result (partial success).
	InspectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

// RelationshipInspector is the OPTIONAL capability to report a namespace's
// foreign-key edges cheaply (no column detail). Relational engines implement it;
// engines without a foreign-key concept do not, and callers report 501 via the
// same type-assertion path used for SchemaInspector.
type RelationshipInspector interface {
	InspectRelationships(ctx context.Context, namespace string) (*RelationshipGraph, error)
}

// DirectoryInspector is the generalized listing capability new engines should
// implement. Legacy SchemaInspector remains available during migration.
type DirectoryInspector interface {
	SchemaSpec() SchemaSpec
	InspectDirectory(ctx context.Context, opts DirectoryOptions) (*Directory, error)
	InspectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

type ScopedRelationshipInspector interface {
	InspectRelationshipsInScope(ctx context.Context, scope ScopePath) (*RelationshipGraph, error)
}

type DirectoryOptions struct {
	Root ScopePath
}

// ScopeDiscoverer is the optional cheap connection-time hierarchy capability.
// It lists scopes only and must not inspect database objects.
type ScopeDiscoverer interface {
	DiscoverScopes(ctx context.Context, request ScopeDiscoveryRequest) (*ScopeDiscovery, error)
}

type ScopeDiscoveryRequest struct {
	Parent ScopePath `json:"parent,omitempty"`
}

type ScopeDiscovery struct {
	Current ScopePath   `json:"current,omitempty"`
	Scopes  []ScopePath `json:"scopes"`
}

type CatalogOptions struct {
	Database  string
	Namespace string
}
