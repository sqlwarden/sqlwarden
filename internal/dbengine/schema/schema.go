// Package schema is the schema inspection domain. It defines the
// SchemaInspector capability an engine implements to report its objects in two
// tiers (a cheap Directory listing and on-demand Object detail), the data model
// those reports use (objects, columns, keys, descriptors), the static SchemaSpec
// describing which object kinds an engine exposes.
package schema

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// ScopeSegment is one typed level in an engine-defined object hierarchy.
// Examples include database, schema, keyspace, and logical_database.
type ScopeSegment struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ScopePath is the canonical, comparable representation of a hierarchical
// object scope. Its JSON representation is []ScopeSegment, while the underlying
// string is safe to use as a map, cache, and persistence key.
type ScopePath string

// NewScopePath builds a canonical path from typed segments.
func NewScopePath(segments ...ScopeSegment) ScopePath {
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		parts = append(parts, url.PathEscape(segment.Kind)+"="+url.PathEscape(segment.Name))
	}
	return ScopePath(strings.Join(parts, "/"))
}

// Segments decodes the path into its typed hierarchy.
func (p ScopePath) Segments() ([]ScopeSegment, error) {
	if p == "" {
		return []ScopeSegment{}, nil
	}
	parts := strings.Split(string(p), "/")
	segments := make([]ScopeSegment, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid scope path segment %q", part)
		}
		kind, err := url.PathUnescape(pair[0])
		if err != nil {
			return nil, fmt.Errorf("decode scope kind: %w", err)
		}
		name, err := url.PathUnescape(pair[1])
		if err != nil {
			return nil, fmt.Errorf("decode scope name: %w", err)
		}
		if kind == "" || name == "" {
			return nil, fmt.Errorf("scope kind and name must not be empty")
		}
		segments = append(segments, ScopeSegment{Kind: kind, Name: name})
	}
	return segments, nil
}

func (p ScopePath) MarshalJSON() ([]byte, error) {
	segments, err := p.Segments()
	if err != nil {
		return nil, err
	}
	return json.Marshal(segments)
}

func (p *ScopePath) UnmarshalJSON(data []byte) error {
	var segments []ScopeSegment
	if err := json.Unmarshal(data, &segments); err != nil {
		return err
	}
	path := NewScopePath(segments...)
	if _, err := path.Segments(); err != nil {
		return err
	}
	*p = path
	return nil
}

// Child returns a new scope path with segment appended.
func (p ScopePath) Child(segment ScopeSegment) ScopePath {
	segments, err := p.Segments()
	if err != nil {
		return NewScopePath(segment)
	}
	return NewScopePath(append(segments, segment)...)
}

// Name returns the last segment name of kind, or an empty string.
func (p ScopePath) Name(kind string) string {
	segments, err := p.Segments()
	if err != nil {
		return ""
	}
	for index := len(segments) - 1; index >= 0; index-- {
		if segments[index].Kind == kind {
			return segments[index].Name
		}
	}
	return ""
}

// Last returns the final segment.
func (p ScopePath) Last() (ScopeSegment, bool) {
	segments, err := p.Segments()
	if err != nil || len(segments) == 0 {
		return ScopeSegment{}, false
	}
	return segments[len(segments)-1], true
}

// With replaces the last segment of kind, or appends it when absent.
func (p ScopePath) With(kind, name string) ScopePath {
	segments, err := p.Segments()
	if err != nil {
		return NewScopePath(ScopeSegment{Kind: kind, Name: name})
	}
	for index := len(segments) - 1; index >= 0; index-- {
		if segments[index].Kind == kind {
			segments[index].Name = name
			return NewScopePath(segments...)
		}
	}
	return NewScopePath(append(segments, ScopeSegment{Kind: kind, Name: name})...)
}

// ObjectRef is the qualified, addressable identity of a database object. It
// replaces bare name strings wherever an object is referenced (including
// foreign-key targets), which is what makes cross-schema references and
// click-to-navigate possible.
type ObjectRef struct {
	Scope ScopePath `json:"scope"`
	Kind  string    `json:"kind"` // table, view, collection, key, function, …
	Name  string    `json:"name"`
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
	Columns     []Column         `json:"columns"`
	PrimaryKey  []string         `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey     `json:"foreign_keys,omitempty"`
	Indexes     []SecondaryIndex `json:"indexes,omitempty"`
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
// object (carrying its scope), which is what enables cross-schema
// click-to-navigate.
type ForeignKey struct {
	Name              string         `json:"name"`
	Columns           []string       `json:"columns"`
	References        ObjectRef      `json:"references"` // qualified target
	ReferencedColumns []string       `json:"referenced_columns"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

// SecondaryIndex is a database index on a relational object. The explicit
// name distinguishes it from Index, the prepared schema metadata lookup.
type SecondaryIndex struct {
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
