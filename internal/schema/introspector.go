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

// CatalogOptions scopes a catalog request. Empty fields mean the driver's
// current/default database and all namespaces.
type CatalogOptions struct {
	Database  string
	Namespace string
}
