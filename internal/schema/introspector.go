package schema

import "context"

// Introspector is the capability a driver implements to report its schema in two
// tiers: a cheap Catalog listing, and on-demand detail for specific objects. It
// is satisfied implicitly (Go structural typing); drivers need not import this
// interface, only the schema types they return.
type Introspector interface {
	// Capabilities is pure/static and must not touch the target database.
	Capabilities() DriverCapabilities
	// IntrospectCatalog lists objects (names + kinds) without columns/keys.
	IntrospectCatalog(ctx context.Context, opts CatalogOptions) (*Catalog, error)
	// IntrospectObjects returns detail only for the requested refs, pushing the
	// ref filter into the underlying query (never fetch-all-then-filter). Refs
	// that do not exist are simply omitted from the result (partial success).
	IntrospectObjects(ctx context.Context, refs []ObjectRef) ([]Object, error)
}

// CatalogOptions scopes a catalog request. Empty fields mean the driver's
// current/default database and all namespaces.
type CatalogOptions struct {
	Database  string
	Namespace string
}
