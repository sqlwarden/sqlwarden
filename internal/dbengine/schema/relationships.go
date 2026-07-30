package schema

// RelationshipGraph is the cheap topology of a single scope.
type RelationshipGraph struct {
	Scope         ScopePath      `json:"scope,omitempty"`
	Relationships []Relationship `json:"relationships"`
}

// Relationship is one typed edge between fully scoped objects.
type Relationship struct {
	Kind              string         `json:"kind,omitempty"`
	Name              string         `json:"name"`
	Source            ObjectRef      `json:"source"`
	Columns           []string       `json:"columns,omitempty"`
	References        ObjectRef      `json:"references,omitempty"`
	ReferencedColumns []string       `json:"referenced_columns,omitempty"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}
