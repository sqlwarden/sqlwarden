package mysql

import (
	"errors"
	"strings"

	omniparser "github.com/bytebase/omni/mysql/parser"
	"github.com/sqlwarden/internal/engine/ddl"
)

func validateMySQLDefaults(request ddl.Request) error {
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
			if err := validateMySQLDefault(*value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMySQLDefault(value string) error {
	invalid := errors.New("default must be a single SQL expression without comments or additional statements")
	if strings.Contains(value, ";") || strings.Contains(value, "--") || strings.Contains(value, "/*") {
		return invalid
	}
	tree, err := omniparser.Parse("SELECT (" + value + ")")
	if err != nil || tree.Len() != 1 {
		return invalid
	}
	return nil
}
