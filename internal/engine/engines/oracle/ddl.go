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
		ddl.OperationAddColumn,
		ddl.OperationAlterColumn,
		ddl.OperationCreateIndex,
	},
	ColumnTypes: []string{
		"NUMBER", "NUMBER(1)", "FLOAT", "BINARY_FLOAT", "BINARY_DOUBLE",
		"VARCHAR2(255)", "VARCHAR2(4000)", "CHAR", "NCHAR", "NVARCHAR2(255)",
		"CLOB", "NCLOB", "BLOB", "RAW(2000)", "LONG", "LONG RAW",
		"ROWID", "UROWID", "XMLTYPE",
		"DATE", "TIMESTAMP", "TIMESTAMP WITH TIME ZONE",
		"TIMESTAMP WITH LOCAL TIME ZONE",
		"INTERVAL YEAR(2) TO MONTH", "INTERVAL DAY(2) TO SECOND(6)",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view", "materialized_view"},
	SupportsCascade:          true,
	SupportsColumnDefaults:   true,
	ParameterizedColumnTypes: []ddl.ParameterizedColumnType{
		{Name: "NUMBER", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 38}, {Name: "scale", Min: -84, Max: 127, Optional: true}}},
		{Name: "VARCHAR2", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 4000}}},
		{Name: "NVARCHAR2", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 2000}}},
		{Name: "CHAR", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 2000}}},
		{Name: "NCHAR", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 1000}}},
		{Name: "RAW", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 2000}}},
		{Name: "FLOAT", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 126}}},
		{Name: "TIMESTAMP", Parameters: []ddl.ColumnTypeParameter{{Name: "fractional seconds", Min: 0, Max: 9}}},
		{Name: "TIMESTAMP", Suffix: "WITH TIME ZONE", Parameters: []ddl.ColumnTypeParameter{{Name: "fractional seconds", Min: 0, Max: 9}}},
		{Name: "TIMESTAMP", Suffix: "WITH LOCAL TIME ZONE", Parameters: []ddl.ColumnTypeParameter{{Name: "fractional seconds", Min: 0, Max: 9}}},
	},
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
	// Identifiers are quoted, types are canonicalized from the capability
	// grammar, and default expressions are parsed within a bounded expression.
	// codeql[go/sql-injection]
	if _, err := d.conn().ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("oracle: apply DDL: %w", err)
	}
	return nil
}

func oracleDDLSQL(request ddl.Request) (string, error) {
	if err := ddl.Validate(request, oracleDDLSpec); err != nil {
		return "", err
	}
	if err := validateOracleDefaults(request); err != nil {
		return "", err
	}
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
	case ddl.OperationAddColumn:
		return "ALTER TABLE " + oracleDDLRef(request) + " ADD (" + oracleDDLColumns([]ddl.ColumnDefinition{*request.Column}) + ")", nil
	case ddl.OperationAlterColumn:
		parts := []string{oracleQuoteIdent(request.Name)}
		if request.Changes.DataType != nil {
			dataType, _ := oracleDDLSpec.CanonicalColumnType(*request.Changes.DataType)
			parts = append(parts, dataType)
		}
		if request.Changes.Default != nil {
			parts = append(parts, "DEFAULT ("+strings.TrimSpace(*request.Changes.Default)+")")
		}
		if request.Changes.Nullable != nil {
			if *request.Changes.Nullable {
				parts = append(parts, "NULL")
			} else {
				parts = append(parts, "NOT NULL")
			}
		}
		return "ALTER TABLE " + oracleDDLRef(request) + " MODIFY (" + strings.Join(parts, " ") + ")", nil
	case ddl.OperationCreateIndex:
		prefix := "CREATE "
		if request.Unique {
			prefix += "UNIQUE "
		}
		columns := make([]string, len(request.IndexColumns))
		for i, column := range request.IndexColumns {
			columns[i] = oracleQuoteIdent(column.Name)
			if column.Descending {
				columns[i] += " DESC"
			}
		}
		return prefix + "INDEX " + oracleQualified(request.Ref.Scope.Name("schema"), request.Name) + " ON " + oracleDDLRef(request) + " (" + strings.Join(columns, ", ") + ")", nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func oracleDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := oracleDDLSpec.CanonicalColumnType(column.DataType)
		definition := oracleQuoteIdent(column.Name) + " " + dataType
		if column.Default != nil {
			definition += " DEFAULT (" + strings.TrimSpace(*column.Default) + ")"
		}
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
