package schema

import "time"

// Schema is a driver-agnostic snapshot of a target database's structure.
type Schema struct {
	Connection  string         `json:"connection"`
	Database    string         `json:"database"`
	GeneratedAt time.Time      `json:"generated_at"`
	Namespaces  []Namespace    `json:"namespaces"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// Namespace is a logical container of database objects (a postgres schema,
// a mysql database, or sqlite's single implicit namespace).
type Namespace struct {
	Name         string         `json:"name"`
	ObjectGroups []ObjectGroup  `json:"object_groups"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// ObjectGroup is an ordered, driver-labelled category of like objects
// (Tables, Views, Functions, …). Kind is a stable machine id; Label is the
// human heading. A new database extends the schema simply by declaring new
// kinds — no change to this type or to generic consumers is required.
type ObjectGroup struct {
	Kind    string   `json:"kind"`
	Label   string   `json:"label"`
	Objects []Object `json:"objects"`
}

// Object is a single database object. Relational kinds (table/view/matview)
// populate Columns and the key/index fields; other kinds carry kind-specific
// data in Attributes. The object's kind lives on the enclosing ObjectGroup.
type Object struct {
	Name        string         `json:"name"`
	Columns     []Column       `json:"columns,omitempty"`
	PrimaryKey  []string       `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey   `json:"foreign_keys,omitempty"`
	Indexes     []Index        `json:"indexes,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
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
	ReferencedTable   string         `json:"referenced_table"`
	ReferencedColumns []string       `json:"referenced_columns"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

type Index struct {
	Name       string         `json:"name"`
	Columns    []string       `json:"columns"`
	Unique     bool           `json:"unique"`
	Attributes map[string]any `json:"attributes,omitempty"`
}
