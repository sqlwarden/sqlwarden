// Package completioncore contains SQLWarden's dialect-neutral completion
// boundary. The dialect implementations are adapted from Bytebase's MIT
// licensed completion implementation, while metadata remains owned by
// SQLWarden.
//
// Keeping parser candidates and catalog resolution behind this package is
// intentional: as Omni grows complete semantic resolvers, a dialect can
// delegate more work to Omni without changing the dbengine or HTTP contracts.
package completioncore

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/dbengine/schema"
)

// CandidateType classifies a semantic completion item independently of a
// parser's native enums.
type CandidateType string

const (
	CandidateKeyword          CandidateType = "keyword"
	CandidateDatabase         CandidateType = "database"
	CandidateSchema           CandidateType = "schema"
	CandidateTable            CandidateType = "table"
	CandidateForeignTable     CandidateType = "foreign_table"
	CandidateView             CandidateType = "view"
	CandidateMaterializedView CandidateType = "materialized_view"
	CandidateColumn           CandidateType = "column"
	CandidateFunction         CandidateType = "function"
	CandidateProcedure        CandidateType = "procedure"
	CandidateSequence         CandidateType = "sequence"
	CandidateIndex            CandidateType = "index"
	CandidateTrigger          CandidateType = "trigger"
	CandidateEvent            CandidateType = "event"
	CandidateCharset          CandidateType = "charset"
	CandidateEngine           CandidateType = "engine"
	CandidateTypeName         CandidateType = "type"
	CandidateVariable         CandidateType = "variable"
)

// Candidate is the transport-neutral result produced by a dialect completion
// engine. Priority follows Bytebase's convention: smaller values rank first.
type Candidate struct {
	Text         string
	DisplayText  string
	Type         CandidateType
	Definition   string
	Comment      string
	Priority     int
	ReplaceStart int
}

// Column is the metadata needed by completion. It intentionally excludes
// keys, indexes, and other schema-inspector details.
type Column struct {
	Name     string
	Type     string
	Nullable bool
	Comment  string
}

// Relation is a table-like object visible to completion.
type Relation struct {
	Database   string
	Schema     string
	Name       string
	Kind       CandidateType
	Definition string
	Columns    []Column
}

// MetadataResolver is the completion-specific view over an immutable schema
// index. Implementations adapt canonical schema metadata into completion
// relations without owning another prepared lookup structure.
// Implementations must not fetch a live database.
type MetadataResolver interface {
	DefaultDatabase() string
	DefaultSchema() string
	DatabaseNames() []string
	SchemaNames(database string) []string
	Relations(database, schema string) []Relation
	FindRelation(database, schema, name string) (Relation, bool)
}

// SchemaResolver adapts schema.Index to the completion metadata boundary.
// The underlying index may represent persistent or ephemeral metadata.
type SchemaResolver struct {
	index         *schema.Index
	defaultSchema string
}

func NewSchemaResolver(index *schema.Index, defaultSchema string) *SchemaResolver {
	return &SchemaResolver{index: index, defaultSchema: defaultSchema}
}

func (r *SchemaResolver) DefaultDatabase() string {
	if r == nil || r.index == nil {
		return ""
	}
	return r.index.DefaultScope().Name("database")
}

func (r *SchemaResolver) DefaultSchema() string {
	if r == nil {
		return ""
	}
	return r.defaultSchema
}

func (r *SchemaResolver) DatabaseNames() []string {
	if r == nil || r.index == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, scope := range r.index.Scopes() {
		if name := scope.Name("database"); name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (r *SchemaResolver) SchemaNames(database string) []string {
	if r == nil || r.index == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, scope := range r.index.Scopes() {
		if !matchesDatabase(database, scope.Name("database")) {
			continue
		}
		if name := scope.Name("schema"); name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (r *SchemaResolver) Relations(database, namespace string) []Relation {
	if r == nil || r.index == nil {
		return nil
	}
	scope, ok := r.resolveScope(database, namespace)
	if !ok {
		return nil
	}
	objects := r.index.ObjectsInScope(scope, "")
	result := make([]Relation, 0, len(objects))
	for _, object := range objects {
		if relation, ok := relationFromObject(object); ok {
			result = append(result, relation)
		}
	}
	return result
}

func (r *SchemaResolver) FindRelation(database, namespace, name string) (Relation, bool) {
	if r == nil || r.index == nil {
		return Relation{}, false
	}
	namespaces := []string{namespace}
	if namespace == "" && r.defaultSchema != "" {
		namespaces = append(namespaces, r.defaultSchema)
	}
	for _, candidateNamespace := range namespaces {
		scope, ok := r.resolveScope(database, candidateNamespace)
		if !ok {
			continue
		}
		for _, kind := range relationKinds {
			object, ok := r.index.FindObjectInScope(scope, kind, name)
			if !ok {
				continue
			}
			return relationFromObject(object)
		}
	}
	return Relation{}, false
}

func (r *SchemaResolver) resolveScope(database, namespace string) (schema.ScopePath, bool) {
	defaultScope := r.index.DefaultScope()
	for _, scope := range r.index.Scopes() {
		if !matchesDatabase(database, scope.Name("database")) {
			continue
		}
		if namespace != "" && !strings.EqualFold(namespace, scope.Name("schema")) &&
			!strings.EqualFold(namespace, scope.Name("database")) {
			continue
		}
		if namespace == "" && defaultScope != "" && scope != defaultScope {
			continue
		}
		return scope, true
	}
	return "", false
}

func matchesDatabase(requested, current string) bool {
	return requested == "" || strings.EqualFold(requested, current)
}

// CheckContext lets CPU-bound completion loops remain cancellation-aware.
func CheckContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

var relationKinds = []string{"table", "foreign_table", "view", "materialized_view"}

func relationFromObject(object schema.Object) (Relation, bool) {
	kind, ok := relationCandidateType(object.Ref.Kind)
	if !ok {
		return Relation{}, false
	}
	relation := Relation{
		Database:   object.Ref.Scope.Name("database"),
		Schema:     object.Ref.Scope.Name("schema"),
		Name:       object.Ref.Name,
		Kind:       kind,
		Definition: objectSource(object),
	}
	if object.Relational != nil {
		relation.Columns = make([]Column, 0, len(object.Relational.Columns))
		for _, column := range object.Relational.Columns {
			relation.Columns = append(relation.Columns, Column{
				Name: column.Name, Type: column.DataType, Nullable: column.Nullable,
				Comment: stringAttribute(column.Attributes, "comment"),
			})
		}
	}
	return relation, true
}

func relationCandidateType(kind string) (CandidateType, bool) {
	switch kind {
	case "table":
		return CandidateTable, true
	case "foreign_table":
		return CandidateForeignTable, true
	case "view":
		return CandidateView, true
	case "materialized_view":
		return CandidateMaterializedView, true
	default:
		return "", false
	}
}

func objectSource(object schema.Object) string {
	for _, descriptor := range object.Descriptors {
		if descriptor.Source != nil && descriptor.Source.Language == "sql" {
			return descriptor.Source.Body
		}
	}
	return ""
}

func stringAttribute(attributes map[string]any, key string) string {
	if attributes == nil {
		return ""
	}
	value, _ := attributes[key].(string)
	return value
}

// ColumnDefinition formats details consistently across dialect adapters.
func ColumnDefinition(relation Relation, column Column) string {
	owner := relation.Name
	if relation.Schema != "" {
		owner = relation.Schema + "." + owner
	}
	result := fmt.Sprintf("%s | %s", owner, column.Type)
	if !column.Nullable {
		result += ", NOT NULL"
	}
	return result
}
