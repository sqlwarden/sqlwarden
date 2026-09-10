package postgres

import (
	"errors"
	"strings"

	omnpg "github.com/bytebase/omni/pg"
	"github.com/sqlwarden/internal/engine/ddl"
)

func validatePostgresDefaults(request ddl.Request) error {
	var values []*string
	switch request.Operation {
	case ddl.OperationCreateTable:
		for _, column := range request.Columns {
			values = append(values, column.Default)
		}
	case ddl.OperationAddColumn:
		values = append(values, request.Column.Default)
	case ddl.OperationAlterColumn:
		values = append(values, request.Changes.Default)
	}
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			if err := validatePostgresDefault(*value); err != nil {
				return err
			}
		}
	}
	return nil
}

// A default is embedded inside parentheses, so it must parse as exactly one
// scalar expression with no statement separator or trailing content.
func validatePostgresDefault(value string) error {
	invalid := errors.New("default must be a single SQL expression without comments or additional statements")
	if strings.Contains(value, ";") || strings.Contains(value, "--") || strings.Contains(value, "/*") {
		return invalid
	}
	statements, err := omnpg.Parse("SELECT (" + value + ")")
	if err != nil || len(statements) != 1 {
		return invalid
	}
	return nil
}
