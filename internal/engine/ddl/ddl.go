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
)

var ErrUnsupported = errors.New("DDL is not supported")

type ColumnDefinition struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
}

// Request is a tagged schema mutation. Fields not used by the selected
// operation must be omitted by clients and are ignored by engines.
type Request struct {
	Operation Operation           `json:"operation"`
	Scope     metadata.ScopePath  `json:"scope,omitempty"`
	Ref       *metadata.ObjectRef `json:"ref,omitempty"`
	Name      string              `json:"name,omitempty"`
	NewName   string              `json:"new_name,omitempty"`
	Columns   []ColumnDefinition  `json:"columns,omitempty"`
	Cascade   bool                `json:"cascade,omitempty"`
}

// Spec is static and safe to expose without opening a target connection.
type Spec struct {
	Operations               []Operation `json:"operations"`
	ColumnTypes              []string    `json:"column_types"`
	CreatableTableScopeKinds []string    `json:"creatable_table_scope_kinds"`
	DroppableObjectKinds     []string    `json:"droppable_object_kinds"`
	DroppableScopeKinds      []string    `json:"droppable_scope_kinds"`
	SupportsCascade          bool        `json:"supports_cascade"`
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
			if _, ok := CanonicalColumnType(column.DataType, spec.ColumnTypes); !ok {
				return fmt.Errorf("column %q has unsupported data type %q", column.Name, column.DataType)
			}
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
		if err := validateTableRef(request.Ref); err != nil {
			return err
		}
		if err := validateObjectScope(request.Ref, spec); err != nil {
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
		if err := validateTableRef(request.Ref); err != nil {
			return err
		}
		if err := validateObjectScope(request.Ref, spec); err != nil {
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

func validateTableRef(ref *metadata.ObjectRef) error {
	if err := validateRef(ref, "table"); err != nil {
		return err
	}
	if ref.Kind != "table" {
		return fmt.Errorf("expected table reference, got %q", ref.Kind)
	}
	return nil
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
