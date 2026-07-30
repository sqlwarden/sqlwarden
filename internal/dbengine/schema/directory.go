package schema

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
}

// ObjectGroup is the set of objects of one kind within a scope.
// Objects is empty for kinds whose Listing is "searched" (too many to enumerate
// up front).
type ObjectGroup struct {
	Kind    string      `json:"kind"`
	Objects []ObjectRef `json:"objects"` // empty for `searched` kinds
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
	Dialect string             `json:"dialect"`
	Kinds   []SchemaObjectKind `json:"kinds"`
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
}
