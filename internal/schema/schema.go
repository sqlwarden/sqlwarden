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

// Namespace is a logical container of tables/views (a postgres schema,
// a mysql database, or sqlite's single implicit namespace).
type Namespace struct {
	Name       string         `json:"name"`
	Tables     []Table        `json:"tables"`
	Views      []View         `json:"views"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type Table struct {
	Name        string         `json:"name"`
	Columns     []Column       `json:"columns"`
	PrimaryKey  []string       `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey   `json:"foreign_keys,omitempty"`
	Indexes     []Index        `json:"indexes,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type View struct {
	Name       string         `json:"name"`
	Columns    []Column       `json:"columns"`
	Attributes map[string]any `json:"attributes,omitempty"`
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
