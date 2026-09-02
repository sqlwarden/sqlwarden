package oracle

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*oracleDriver)(nil)

var oracleDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes: []string{
		"NUMBER", "NUMBER(1)", "FLOAT", "BINARY_FLOAT", "BINARY_DOUBLE",
		"VARCHAR2(255)", "VARCHAR2(4000)", "CHAR", "NCHAR", "NVARCHAR2(255)",
		"CLOB", "NCLOB", "BLOB", "RAW(2000)",
		"DATE", "TIMESTAMP", "TIMESTAMP WITH TIME ZONE",
		"TIMESTAMP WITH LOCAL TIME ZONE",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view", "materialized_view"},
	SupportsCascade:          true,
}

func (d *oracleDriver) DDLSpec() ddl.Spec { return oracleDDLSpec }

func (d *oracleDriver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, oracleDDLSpec); err != nil {
		return err
	}
	statement, err := oracleDDLSQL(request)
	if err != nil {
		return err
	}
	// Every interpolated value is oracleQuoteIdent-escaped or a data type from
	// the closed oracleDDLSpec.ColumnTypes allowlist.
	// codeql[go/sql-injection]
	if _, err := d.conn().ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("oracle: apply DDL: %w", err)
	}
	return nil
}

func oracleDDLSQL(request ddl.Request) (string, error) {
	switch request.Operation {
	case ddl.OperationCreateTable:
		return "CREATE TABLE " +
			oracleQualified(request.Scope.Name("schema"), request.Name) +
			" (" + oracleDDLColumns(request.Columns) + ")", nil
	case ddl.OperationDropObject:
		switch request.Ref.Kind {
		case "table":
			sql := "DROP TABLE " + oracleDDLRef(request)
			if request.Cascade {
				sql += " CASCADE CONSTRAINTS"
			}
			return sql, nil
		case "view":
			return "DROP VIEW " + oracleDDLRef(request), nil
		case "materialized_view":
			return "DROP MATERIALIZED VIEW " + oracleDDLRef(request), nil
		default:
			return "", fmt.Errorf("%w: drop object kind %q", ddl.ErrUnsupported, request.Ref.Kind)
		}
	case ddl.OperationRenameColumn:
		return "ALTER TABLE " + oracleDDLRef(request) +
			" RENAME COLUMN " + oracleQuoteIdent(request.Name) +
			" TO " + oracleQuoteIdent(request.NewName), nil
	case ddl.OperationDropColumn:
		return "ALTER TABLE " + oracleDDLRef(request) +
			" DROP COLUMN " + oracleQuoteIdent(request.Name), nil
	case ddl.OperationDropIndex:
		return "DROP INDEX " +
			oracleQualified(request.Ref.Scope.Name("schema"), request.Name), nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func oracleDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := ddl.CanonicalColumnType(column.DataType, oracleDDLSpec.ColumnTypes)
		definition := oracleQuoteIdent(column.Name) + " " + dataType
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
		if column.PrimaryKey {
			primary = append(primary, oracleQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func oracleDDLRef(request ddl.Request) string {
	return oracleQualified(request.Ref.Scope.Name("schema"), request.Ref.Name)
}
