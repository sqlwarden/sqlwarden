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

// Catalog is the legacy one-database listing retained during the compatibility
// migration. Services convert it to Directory at their boundaries.
type Catalog struct {
	Connection       string             `json:"connection"`
	Dialect          string             `json:"dialect"`
	Database         string             `json:"database"`
	DefaultNamespace string             `json:"default_namespace,omitempty"`
	GeneratedAt      time.Time          `json:"generated_at"`
	Namespaces       []NamespaceCatalog `json:"namespaces"`
}

type NamespaceCatalog struct {
	Name   string               `json:"name"`
	Groups []ObjectGroupCatalog `json:"groups"`
}

// DirectoryFromCatalog converts the current one-database representation to
// the generalized hierarchy without loading object detail.
func DirectoryFromCatalog(catalog *Catalog) *Directory {
	if catalog == nil {
		return nil
	}
	if catalog.Dialect == "mysql" || catalog.Dialect == "sqlite" {
		directory := &Directory{
			Connection:  catalog.Connection,
			Engine:      catalog.Dialect,
			GeneratedAt: catalog.GeneratedAt,
		}
		for _, namespace := range catalog.Namespaces {
			scope := NewScopePath(ScopeSegment{Kind: "database", Name: namespace.Name})
			directory.Roots = append(directory.Roots, ScopeNode{
				Path:   scope,
				Groups: scopedCatalogGroups(namespace.Groups, scope),
			})
		}
		if len(directory.Roots) == 0 && catalog.Database != "" {
			directory.Roots = append(directory.Roots, ScopeNode{
				Path: NewScopePath(ScopeSegment{Kind: "database", Name: catalog.Database}),
			})
		}
		defaultDatabase := catalog.DefaultNamespace
		if defaultDatabase == "" {
			defaultDatabase = catalog.Database
		}
		if defaultDatabase != "" {
			directory.DefaultScope = NewScopePath(ScopeSegment{Kind: "database", Name: defaultDatabase})
		}
		return directory
	}
	root := NewScopePath(ScopeSegment{Kind: "database", Name: catalog.Database})
	directory := &Directory{
		Connection:  catalog.Connection,
		Engine:      catalog.Dialect,
		GeneratedAt: catalog.GeneratedAt,
		Roots:       []ScopeNode{{Path: root}},
	}
	for _, namespace := range catalog.Namespaces {
		scope := root.Child(ScopeSegment{Kind: "schema", Name: namespace.Name})
		node := ScopeNode{Path: scope, Groups: scopedCatalogGroups(namespace.Groups, scope)}
		directory.Roots[0].Children = append(directory.Roots[0].Children, node)
	}
	if catalog.DefaultNamespace != "" {
		directory.DefaultScope = root.Child(ScopeSegment{Kind: "schema", Name: catalog.DefaultNamespace})
	} else {
		directory.DefaultScope = root
	}
	return directory
}

func scopedCatalogGroups(groups []ObjectGroupCatalog, scope ScopePath) []ObjectGroupCatalog {
	result := make([]ObjectGroupCatalog, len(groups))
	for groupIndex, group := range groups {
		result[groupIndex] = group
		result[groupIndex].Objects = make([]ObjectRef, len(group.Objects))
		for refIndex, ref := range group.Objects {
			ref.Scope = scope
			result[groupIndex].Objects[refIndex] = ref
		}
	}
	return result
}

// ScopeNode is one node in an engine-defined scope hierarchy.
type ScopeNode struct {
	Path     ScopePath            `json:"path"`
	Groups   []ObjectGroupCatalog `json:"groups"`
	Children []ScopeNode          `json:"children,omitempty"`
}

// ObjectGroupCatalog is the set of objects of one kind within a namespace.
// Objects is empty for kinds whose Listing is "searched" (too many to enumerate
// up front).
type ObjectGroupCatalog struct {
	Kind    string      `json:"kind"`
	Objects []ObjectRef `json:"objects"` // empty for `searched` kinds
}

// SchemaSpec is a driver's static declaration of the object kinds it
// exposes, mirroring the permission catalog as the backend source of truth for
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
