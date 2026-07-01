package schema

// RelationshipGraph is the cheap, columns-free foreign-key topology of a single
// namespace: every FK edge between objects, without any column/index detail.
// It stays small even on databases with thousands of tables.
type RelationshipGraph struct {
	Namespace     string         `json:"namespace"`
	Relationships []Relationship `json:"relationships"`
}

// Relationship is one foreign-key edge: the owning (source) object and columns,
// and the referenced (target) object and columns. Both refs are qualified.
type Relationship struct {
	Name              string    `json:"name"`
	Source            ObjectRef `json:"source"`
	Columns           []string  `json:"columns"`
	References        ObjectRef `json:"references"`
	ReferencedColumns []string  `json:"referenced_columns"`
}
