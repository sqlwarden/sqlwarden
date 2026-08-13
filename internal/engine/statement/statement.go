// Package statement defines the optional engine capability for generating
// dialect-specific SQL statement templates from inspected object metadata.
package statement

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// Operation identifies a statement template the engine can generate.
type Operation string

const (
	OperationSelect Operation = "select"
	OperationInsert Operation = "insert"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

var ErrUnsupported = errors.New("statement generation is not supported")

// ObjectSpec describes the operations available for one metadata object kind.
type ObjectSpec struct {
	Kind       string      `json:"kind"`
	Operations []Operation `json:"operations"`
}

// Spec is the static statement-generation capability advertised by an engine.
type Spec struct {
	Objects []ObjectSpec `json:"objects"`
}

// Request contains the inspected object and requested template operation.
type Request struct {
	Operation Operation
	Object    metadata.Object
}

// Generator advertises and produces dialect-specific SQL templates. Generate
// is pure: it uses supplied metadata and never executes SQL or opens a connection.
type Generator interface {
	// StatementSpec reports the operations Generate accepts for each object kind.
	StatementSpec() Spec
	// Generate returns SQL text for request without executing it.
	Generate(Request) (string, error)
}

// OperationsFor returns a copy of the operations supported for kind.
func (s Spec) OperationsFor(kind string) []Operation {
	for _, object := range s.Objects {
		if object.Kind == kind {
			return append([]Operation(nil), object.Operations...)
		}
	}
	return nil
}

// Supports reports whether operation is advertised for kind.
func (s Spec) Supports(kind string, operation Operation) bool {
	for _, candidate := range s.OperationsFor(kind) {
		if candidate == operation {
			return true
		}
	}
	return false
}

// Validate checks the engine-independent request shape and advertised spec.
func Validate(request Request, spec Spec) error {
	if !spec.Supports(request.Object.Ref.Kind, request.Operation) {
		return fmt.Errorf("%w: %s for object kind %q", ErrUnsupported, request.Operation, request.Object.Ref.Kind)
	}
	if request.Object.Ref.Scope == "" {
		return errors.New("object scope is required")
	}
	segments, err := request.Object.Ref.Scope.Segments()
	if err != nil || len(segments) == 0 {
		return errors.New("object scope is invalid")
	}
	if strings.TrimSpace(request.Object.Ref.Name) == "" {
		return errors.New("object name is required")
	}
	if request.Object.Relational == nil {
		return errors.New("relational object detail is required")
	}
	if len(request.Object.Relational.Columns) == 0 {
		return errors.New("at least one column is required")
	}
	for _, column := range request.Object.Relational.Columns {
		if strings.TrimSpace(column.Name) == "" {
			return errors.New("column name is required")
		}
	}
	return nil
}

// Build formats a validated statement from dialect-quoted identifiers and
// dialect-specific value placeholders. Engines retain ownership of quoting.
func Build(operation Operation, qualified string, columns, values []string) (string, error) {
	if qualified == "" {
		return "", errors.New("qualified object name is required")
	}
	if len(columns) == 0 {
		return "", errors.New("at least one column is required")
	}
	switch operation {
	case OperationSelect:
		return "SELECT\n  " + strings.Join(columns, ",\n  ") + "\nFROM " + qualified + ";", nil
	case OperationInsert:
		if len(values) != len(columns) {
			return "", errors.New("one value placeholder is required per column")
		}
		return "INSERT INTO " + qualified + " (\n  " + strings.Join(columns, ",\n  ") + "\n)\nVALUES (\n  " + strings.Join(values, ",\n  ") + "\n);", nil
	case OperationUpdate:
		if len(values) != len(columns) {
			return "", errors.New("one value placeholder is required per column")
		}
		assignments := make([]string, len(columns))
		for index := range columns {
			assignments[index] = columns[index] + " = " + values[index]
		}
		return "UPDATE " + qualified + "\nSET\n  " + strings.Join(assignments, ",\n  ") + "\nWHERE 1 = 0;", nil
	case OperationDelete:
		return "DELETE FROM " + qualified + "\nWHERE 1 = 0;", nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupported, operation)
	}
}
