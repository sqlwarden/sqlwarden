// Package schema is the schema-introspection domain. It defines the
// SchemaInspector capability an engine implements to report its objects in two
// tiers (a cheap Catalog listing and on-demand Object detail), the data model
// those reports use (objects, columns, keys, descriptors), the static SchemaSpec
// describing which object kinds an engine exposes, and a caching Service that
// serves catalogs and object detail to the IDE's schema tree.
package schema

// ObjectRef is the qualified, addressable identity of a database object. It
// replaces bare name strings wherever an object is referenced (including
// foreign-key targets), which is what makes cross-schema references and
// click-to-navigate possible.
type ObjectRef struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // table, view, function, …
	Name      string `json:"name"`
}

// Object is the on-demand detail for a single database object. Known relational
// kinds populate the typed Relational facet; any other (or unknown) kind carries
// self-describing Descriptors. A relational object never duplicates its columns
// into Descriptors — the two facets are disjoint by construction.
type Object struct {
	Ref         ObjectRef         `json:"ref"`
	Relational  *RelationalDetail `json:"relational,omitempty"`
	Descriptors []Descriptor      `json:"descriptors,omitempty"`
	Attributes  map[string]any    `json:"attributes,omitempty"`
}

// RelationalDetail is the typed structure of a relational object (table or
// view): its columns, primary key, foreign keys, and indexes.
type RelationalDetail struct {
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
}

// Column is one column of a relational object. Ordinal is its position; engine-
// specific extras live in Attributes.
type Column struct {
	Name       string         `json:"name"`
	DataType   string         `json:"data_type"`
	Nullable   bool           `json:"nullable"`
	Default    *string        `json:"default,omitempty"`
	Ordinal    int            `json:"ordinal"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ForeignKey is a foreign-key constraint. References is the qualified target
// object (carrying its namespace), which is what enables cross-schema
// click-to-navigate.
type ForeignKey struct {
	Name              string         `json:"name"`
	Columns           []string       `json:"columns"`
	References        ObjectRef      `json:"references"` // qualified target
	ReferencedColumns []string       `json:"referenced_columns"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

// Index is a secondary index on a relational object.
type Index struct {
	Name       string         `json:"name"`
	Columns    []string       `json:"columns"`
	Unique     bool           `json:"unique"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Descriptor is a self-describing piece of an object's structure, named by data
// shape (not by widget). Exactly one of Fields/Rows/Source is set per Kind.
type Descriptor struct {
	Kind   string  `json:"kind"` // "fields" | "rows" | "source"
	Title  string  `json:"title"`
	Fields []Field `json:"fields,omitempty"`
	Rows   *RowSet `json:"rows,omitempty"`
	Source *Source `json:"source,omitempty"`
}

// Field is a single name/value pair in a "fields" Descriptor (e.g. a sequence's
// current value or a function's return type).
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RowSet is a small tabular payload in a "rows" Descriptor (e.g. a trigger's
// timing/event columns).
type RowSet struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// Source is a code body in a "source" Descriptor (e.g. a view or function
// definition) with its language for syntax highlighting.
type Source struct {
	Language string `json:"language"`
	Body     string `json:"body"`
}
