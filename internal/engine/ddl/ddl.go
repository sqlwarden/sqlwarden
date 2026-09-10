// Package ddl defines the optional engine capability for structured
// schema changes initiated outside the SQL editor.
package ddl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

type Operation string

const (
	OperationCreateTable  Operation = "create_table"
	OperationDropObject   Operation = "drop_object"
	OperationDropScope    Operation = "drop_scope"
	OperationRenameColumn Operation = "rename_column"
	OperationDropColumn   Operation = "drop_column"
	OperationDropIndex    Operation = "drop_index"
	OperationAddColumn    Operation = "add_column"
	OperationAlterColumn  Operation = "alter_column"
	OperationCreateIndex  Operation = "create_index"
)

var ErrUnsupported = errors.New("DDL is not supported")

type ColumnDefinition struct {
	Name       string  `json:"name"`
	DataType   string  `json:"data_type"`
	Nullable   bool    `json:"nullable"`
	PrimaryKey bool    `json:"primary_key"`
	Default    *string `json:"default,omitempty"`
}

// ColumnChanges is a patch: absent fields preserve the current definition.
// A default expression of NULL removes the effective default.
type ColumnChanges struct {
	DataType *string `json:"data_type,omitempty"`
	Nullable *bool   `json:"nullable,omitempty"`
	Default  *string `json:"default,omitempty"`
}

type IndexColumn struct {
	Name       string `json:"name"`
	Descending bool   `json:"descending,omitempty"`
}

// Request is a tagged schema mutation. Fields not used by the selected
// operation must be omitted by clients and are ignored by engines.
type Request struct {
	Operation    Operation           `json:"operation"`
	Scope        metadata.ScopePath  `json:"scope,omitempty"`
	Ref          *metadata.ObjectRef `json:"ref,omitempty"`
	Name         string              `json:"name,omitempty"`
	NewName      string              `json:"new_name,omitempty"`
	Columns      []ColumnDefinition  `json:"columns,omitempty"`
	Cascade      bool                `json:"cascade,omitempty"`
	Column       *ColumnDefinition   `json:"column,omitempty"`
	Changes      *ColumnChanges      `json:"changes,omitempty"`
	IndexColumns []IndexColumn       `json:"index_columns,omitempty"`
	Unique       bool                `json:"unique,omitempty"`
}

// Spec is static and safe to expose without opening a target connection.
type Spec struct {
	Operations               []Operation               `json:"operations"`
	ColumnTypes              []string                  `json:"column_types"`
	CreatableTableScopeKinds []string                  `json:"creatable_table_scope_kinds"`
	DroppableObjectKinds     []string                  `json:"droppable_object_kinds"`
	DroppableScopeKinds      []string                  `json:"droppable_scope_kinds"`
	SupportsCascade          bool                      `json:"supports_cascade"`
	SupportsColumnDefaults   bool                      `json:"supports_column_defaults,omitempty"`
	ParameterizedColumnTypes []ParameterizedColumnType `json:"parameterized_column_types,omitempty"`
	// AllowCustomColumnTypes permits column type text outside ColumnTypes and
	// ParameterizedColumnTypes, for engines whose installed extensions can add
	// arbitrary types (e.g. Postgres extensions like pgvector or PostGIS).
	// CanonicalColumnType only checks that the text is syntactically safe to
	// interpolate; the database is the authority on whether the type exists.
	AllowCustomColumnTypes bool `json:"allow_custom_column_types,omitempty"`
}

// Executor advertises and applies a bounded set of structured DDL operations.
// Implementations must validate requests and safely quote every identifier.
type Executor interface {
	// DDLSpec reports the operations and input vocabulary accepted by ApplyDDL.
	DDLSpec() Spec
	// ApplyDDL validates and executes one structured DDL request.
	ApplyDDL(context.Context, Request) error
}

func (s Spec) Supports(operation Operation) bool {
	for _, candidate := range s.Operations {
		if candidate == operation {
			return true
		}
	}
	return false
}

func (s Spec) SupportsObjectKind(kind string) bool {
	return contains(s.DroppableObjectKinds, kind)
}

func (s Spec) SupportsScopeKind(kind string) bool {
	return contains(s.DroppableScopeKinds, kind)
}

// Summary renders a short, human-readable label for the request, for display
// in places (e.g. the pending-statements list of an open transaction) that
// show what ran without exposing the driver's generated SQL.
func (r Request) Summary() string {
	switch r.Operation {
	case OperationCreateTable:
		return fmt.Sprintf("CREATE TABLE %s", r.Name)
	case OperationDropObject:
		if r.Ref != nil {
			return fmt.Sprintf("DROP %s %s", strings.ToUpper(r.Ref.Kind), r.Ref.Name)
		}
		return "DROP OBJECT"
	case OperationDropScope:
		last, _ := r.Scope.Last()
		return fmt.Sprintf("DROP %s", last.Name)
	case OperationRenameColumn:
		if r.Ref != nil {
			return fmt.Sprintf("RENAME COLUMN %s.%s TO %s", r.Ref.Name, r.Name, r.NewName)
		}
		return fmt.Sprintf("RENAME COLUMN %s TO %s", r.Name, r.NewName)
	case OperationDropColumn:
		if r.Ref != nil {
			return fmt.Sprintf("DROP COLUMN %s.%s", r.Ref.Name, r.Name)
		}
		return fmt.Sprintf("DROP COLUMN %s", r.Name)
	case OperationDropIndex:
		return fmt.Sprintf("DROP INDEX %s", r.Name)
	case OperationCreateIndex:
		return fmt.Sprintf("CREATE INDEX %s", r.Name)
	case OperationAddColumn:
		if r.Ref != nil && r.Column != nil {
			return fmt.Sprintf("ADD COLUMN %s.%s", r.Ref.Name, r.Column.Name)
		}
		return "ADD COLUMN"
	case OperationAlterColumn:
		if r.Ref != nil {
			return fmt.Sprintf("ALTER COLUMN %s.%s", r.Ref.Name, r.Name)
		}
		return "ALTER COLUMN"
	default:
		return string(r.Operation)
	}
}

// Validate checks the engine-independent shape and the advertised capability
// contract. Engines remain responsible for dialect-specific validation.
func Validate(request Request, spec Spec) error {
	if !spec.Supports(request.Operation) {
		return fmt.Errorf("%w: operation %q", ErrUnsupported, request.Operation)
	}
	switch request.Operation {
	case OperationCreateTable:
		if err := validateScope(request.Scope); err != nil {
			return err
		}
		last, _ := request.Scope.Last()
		if !contains(spec.CreatableTableScopeKinds, last.Kind) {
			return fmt.Errorf("%w: tables in scope kind %q", ErrUnsupported, last.Kind)
		}
		if err := ValidateIdentifier(request.Name, "table name"); err != nil {
			return err
		}
		if len(request.Columns) == 0 {
			return errors.New("at least one column is required")
		}
		seen := make(map[string]struct{}, len(request.Columns))
		for index, column := range request.Columns {
			if err := ValidateIdentifier(column.Name, fmt.Sprintf("column %d name", index+1)); err != nil {
				return err
			}
			folded := strings.ToLower(column.Name)
			if _, exists := seen[folded]; exists {
				return fmt.Errorf("column name %q is duplicated", column.Name)
			}
			seen[folded] = struct{}{}
			if _, ok := spec.CanonicalColumnType(column.DataType); !ok {
				return fmt.Errorf("column %q has unsupported data type %q", column.Name, column.DataType)
			}
			if err := validateDefault(column.Default, spec); err != nil {
				return err
			}
		}
	case OperationAddColumn:
		if err := validateTableRef(request.Ref, spec); err != nil {
			return err
		}
		if request.Column == nil {
			return errors.New("column definition is required")
		}
		if err := ValidateIdentifier(request.Column.Name, "column name"); err != nil {
			return err
		}
		if _, ok := spec.CanonicalColumnType(request.Column.DataType); !ok {
			return errors.New("unsupported column data type")
		}
		if err := validateDefault(request.Column.Default, spec); err != nil {
			return err
		}
	case OperationAlterColumn:
		if err := validateTableRef(request.Ref, spec); err != nil {
			return err
		}
		if err := ValidateIdentifier(request.Name, "column name"); err != nil {
			return err
		}
		changes := request.Changes
		if changes == nil || (changes.DataType == nil && changes.Nullable == nil && changes.Default == nil) {
			return errors.New("at least one column change is required")
		}
		if changes.DataType != nil {
			if _, ok := spec.CanonicalColumnType(*changes.DataType); !ok {
				return errors.New("unsupported column data type")
			}
		}
		if err := validateDefault(changes.Default, spec); err != nil {
			return err
		}
	case OperationCreateIndex:
		if err := validateTableRef(request.Ref, spec); err != nil {
			return err
		}
		if err := ValidateIdentifier(request.Name, "index name"); err != nil {
			return err
		}
		if len(request.IndexColumns) == 0 {
			return errors.New("at least one index column is required")
		}
		seen := make(map[string]bool, len(request.IndexColumns))
		for _, column := range request.IndexColumns {
			if err := ValidateIdentifier(column.Name, "index column name"); err != nil {
				return err
			}
			if seen[column.Name] {
				return fmt.Errorf("index column %q is duplicated", column.Name)
			}
			seen[column.Name] = true
		}
	case OperationDropObject:
		if err := validateRef(request.Ref, "object"); err != nil {
			return err
		}
		if err := validateObjectScope(request.Ref, spec); err != nil {
			return err
		}
		if !spec.SupportsObjectKind(request.Ref.Kind) {
			return fmt.Errorf("%w: object kind %q", ErrUnsupported, request.Ref.Kind)
		}
	case OperationDropScope:
		if err := validateScope(request.Scope); err != nil {
			return err
		}
		last, _ := request.Scope.Last()
		if !spec.SupportsScopeKind(last.Kind) {
			return fmt.Errorf("%w: scope kind %q", ErrUnsupported, last.Kind)
		}
	case OperationRenameColumn:
		if err := validateTableRef(request.Ref, spec); err != nil {
			return err
		}
		if err := ValidateIdentifier(request.Name, "column name"); err != nil {
			return err
		}
		if err := ValidateIdentifier(request.NewName, "new column name"); err != nil {
			return err
		}
		if request.Name == request.NewName {
			return errors.New("new column name must be different")
		}
	case OperationDropColumn, OperationDropIndex:
		if err := validateTableRef(request.Ref, spec); err != nil {
			return err
		}
		if err := ValidateIdentifier(request.Name, "name"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: operation %q", ErrUnsupported, request.Operation)
	}
	if request.Cascade && !spec.SupportsCascade {
		return fmt.Errorf("%w: cascade", ErrUnsupported)
	}
	return nil
}

// validateDefault checks the defaults capability and rejects empty expressions
// or NUL. Each driver remains responsible for validating its SQL grammar.
func validateDefault(value *string, spec Spec) error {
	if value == nil {
		return nil
	}
	if !spec.SupportsColumnDefaults {
		return fmt.Errorf("%w: column defaults", ErrUnsupported)
	}
	if strings.TrimSpace(*value) == "" || strings.ContainsRune(*value, '\x00') {
		return errors.New("default expression must not be empty or contain NUL")
	}
	return nil
}

func validateObjectScope(ref *metadata.ObjectRef, spec Spec) error {
	last, ok := ref.Scope.Last()
	if !ok || !contains(spec.CreatableTableScopeKinds, last.Kind) {
		return fmt.Errorf("%w: objects in scope kind %q", ErrUnsupported, last.Kind)
	}
	return nil
}

func ValidateIdentifier(value, label string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not be empty or have surrounding whitespace", label)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must not contain NUL", label)
	}
	return nil
}

func CanonicalColumnType(value string, supported []string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	for _, candidate := range supported {
		if strings.EqualFold(trimmed, candidate) {
			return candidate, true
		}
	}
	return "", false
}

func validateRef(ref *metadata.ObjectRef, label string) error {
	if ref == nil {
		return fmt.Errorf("%s reference is required", label)
	}
	if err := validateScope(ref.Scope); err != nil {
		return err
	}
	if ref.Kind == "" {
		return fmt.Errorf("%s kind is required", label)
	}
	return ValidateIdentifier(ref.Name, label+" name")
}

// validateTableRef checks a table's identity and the driver's supported scope
// kinds before operation-specific validation.
func validateTableRef(ref *metadata.ObjectRef, spec Spec) error {
	if err := validateRef(ref, "table"); err != nil {
		return err
	}
	if ref.Kind != "table" {
		return fmt.Errorf("expected table reference, got %q", ref.Kind)
	}
	return validateObjectScope(ref, spec)
}

func validateScope(scope metadata.ScopePath) error {
	if scope == "" {
		return errors.New("scope is required")
	}
	segments, err := scope.Segments()
	if err != nil {
		return fmt.Errorf("invalid scope: %w", err)
	}
	if len(segments) == 0 {
		return errors.New("scope is required")
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
