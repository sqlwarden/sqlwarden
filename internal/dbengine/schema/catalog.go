package schema

import "time"

// Catalog is the cheap, listing-only view of a connection's objects: names and
// kinds grouped by namespace, with no columns/keys/indexes. It stays small even
// on databases with thousands of objects.
type Catalog struct {
	Connection  string             `json:"connection"`
	Dialect     string             `json:"dialect"`
	Database    string             `json:"database"`
	GeneratedAt time.Time          `json:"generated_at"`
	Namespaces  []NamespaceCatalog `json:"namespaces"`
}

type NamespaceCatalog struct {
	Name   string               `json:"name"`
	Groups []ObjectGroupCatalog `json:"groups"`
}

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

type SchemaObjectKind struct {
	Kind            string `json:"kind"`
	Label           string `json:"label"`
	PluralLabel     string `json:"plural_label"`
	Order           int    `json:"order"`
	Relational      bool   `json:"relational"`
	SupportsDiagram bool   `json:"supports_diagram"`
	Listing         string `json:"listing"` // "enumerated" | "searched"
}
