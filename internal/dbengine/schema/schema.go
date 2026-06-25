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

type RelationalDetail struct {
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
}

type Column struct {
	Name       string         `json:"name"`
	DataType   string         `json:"data_type"`
	Nullable   bool           `json:"nullable"`
	Default    *string        `json:"default,omitempty"`
	Ordinal    int            `json:"ordinal"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type ForeignKey struct {
	Name              string         `json:"name"`
	Columns           []string       `json:"columns"`
	References        ObjectRef      `json:"references"` // qualified target
	ReferencedColumns []string       `json:"referenced_columns"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

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

type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type RowSet struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

type Source struct {
	Language string `json:"language"`
	Body     string `json:"body"`
}
