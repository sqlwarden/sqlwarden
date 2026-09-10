package metadata

import "time"

// Directory is the cheap, listing-only view of a connection's objects: names
// and kinds grouped by hierarchical scope, with no columns/keys/indexes.
type Directory struct {
	Connection   string      `json:"connection"`
	Engine       string      `json:"engine"`
	DefaultScope ScopePath   `json:"default_scope"`
	GeneratedAt  time.Time   `json:"generated_at"`
	Roots        []ScopeNode `json:"roots"`
}

// ScopeNode is one node in an engine-defined scope hierarchy.
type ScopeNode struct {
	Path     ScopePath     `json:"path"`
	Groups   []ObjectGroup `json:"groups"`
	Children []ScopeNode   `json:"children,omitempty"`
	Lazy     bool          `json:"lazy,omitempty"`
	System   bool          `json:"system,omitempty"`
}

// ObjectGroup is the set of objects of one kind within a scope.
// Objects is empty for kinds whose Listing is "searched" (too many to enumerate
// up front).
type ObjectGroup struct {
	Kind    string      `json:"kind"`
	Objects []ObjectRef `json:"objects"` // empty for `searched` kinds
	// RowCounts is an optional, driver-supplied approximate row count per
	// object name, populated only for kinds where it's cheap to obtain
	// alongside the listing query (e.g. table/materialized_view via catalog
	// stats). Nil when the driver doesn't support it; objects with no entry
	// simply have no known count.
	RowCounts map[string]int64 `json:"row_counts,omitempty"`
}

// ScopeNodes returns every scope node in stable depth-first directory order.
func (d *Directory) ScopeNodes() []ScopeNode {
	if d == nil {
		return nil
	}
	var result []ScopeNode
	var walk func([]ScopeNode)
	walk = func(nodes []ScopeNode) {
		for _, node := range nodes {
			result = append(result, node)
			walk(node.Children)
		}
	}
	walk(d.Roots)
	return result
}

// WithSystemScopes returns a copy of the directory with system scopes
// removed when show is false. The current default scope is always kept,
// even if it is a system scope, so a connection scoped into a system schema
// doesn't lose its own view.
func (d *Directory) WithSystemScopes(show bool) *Directory {
	if d == nil || show {
		return d
	}
	filtered := *d
	filtered.Roots = filterSystemScopes(d.Roots, d.DefaultScope)
	return &filtered
}

func filterSystemScopes(nodes []ScopeNode, current ScopePath) []ScopeNode {
	if len(nodes) == 0 {
		return nodes
	}
	kept := make([]ScopeNode, 0, len(nodes))
	for _, node := range nodes {
		if node.System && node.Path != current {
			continue
		}
		if node.Children != nil {
			node.Children = filterSystemScopes(node.Children, current)
		}
		kept = append(kept, node)
	}
	return kept
}

// ObjectRefs returns every lightweight object reference in directory order.
func (d *Directory) ObjectRefs() []ObjectRef {
	var refs []ObjectRef
	for _, node := range d.ScopeNodes() {
		for _, group := range node.Groups {
			refs = append(refs, group.Objects...)
		}
	}
	return refs
}

// SchemaSpec is a driver's static declaration of the object kinds it
// exposes, mirroring the permission model as the backend source of truth for
// labels/ordering/flags. The frontend renders generically from it.
type SchemaSpec struct {
	Dialect      string             `json:"dialect"`
	Kinds        []SchemaObjectKind `json:"kinds"`
	BrowseScopes bool               `json:"browse_scopes,omitempty"`
	// SystemSchemas is set when the driver marks built-in/system scopes via
	// ScopeNode.System, so the connection-level "show system schemas" setting
	// has an effect. Drivers that exclude system schemas at the query level
	// instead of flagging them leave this unset.
	SystemSchemas bool `json:"system_schemas,omitempty"`
}

// SchemaObjectKind describes one object kind an engine exposes: its labels and
// display order, whether it is relational (has the typed column/key detail) or
// supports an ER diagram, and how it is listed ("enumerated" up front, or
// "searched" on demand).
type SchemaObjectKind struct {
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	PluralLabel     string `json:"plural_label"`
	Order           int    `json:"order"`
	Relational      bool   `json:"relational"`
	SupportsDiagram bool   `json:"supports_diagram"`
	Listing         string `json:"listing"` // "enumerated" | "searched"
	// HasDefinition marks a kind whose object detail carries a canonical DDL or
	// definition text — either inlined by InspectObjects as a "source" descriptor
	// or served on demand by InspectDefinition. The object viewer uses it to
	// decide whether to offer a DDL tab, so a kind whose definition cannot be
	// reconstructed (e.g. Postgres type/domain) does not show an empty one.
	HasDefinition bool `json:"has_definition,omitempty"`
}
