package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*Driver)(nil)

var mysqlDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationDropScope,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes: []string{
		"bigint", "blob", "boolean", "date", "datetime", "decimal(10,2)", "double",
		"float", "int", "json", "mediumint", "smallint", "text", "time", "timestamp",
		"tinyint", "varbinary(255)", "varchar(255)",
	},
	CreatableTableScopeKinds: []string{"database"},
	DroppableObjectKinds:     []string{"table", "view"},
	DroppableScopeKinds:      []string{"database"},
}

func (d *Driver) DDLSpec() ddl.Spec {
	return mysqlDDLSpec
}

func (d *Driver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, mysqlDDLSpec); err != nil {
		return err
	}
	statement, err := mysqlDDLSQL(request)
	if err != nil {
		return err
	}
	// Every dynamic value is escaped by mysqlQuoteIdent or selected from the
	// closed data-type allowlist above.
	// codeql[go/sql-injection]
	if _, err := d.conn().ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("mysql: apply DDL: %w", err)
	}
	return nil
}

func mysqlDDLSQL(request ddl.Request) (string, error) {
	switch request.Operation {
	case ddl.OperationCreateTable:
		return "CREATE TABLE " + mysqlQuoteQualified(request.Scope.Name("database"), request.Name) + " (" + mysqlDDLColumns(request.Columns) + ")", nil
	case ddl.OperationDropObject:
		verb := map[string]string{"table": "DROP TABLE", "view": "DROP VIEW"}[request.Ref.Kind]
		return verb + " " + mysqlQuoteQualified(request.Ref.Scope.Name("database"), request.Ref.Name), nil
	case ddl.OperationDropScope:
		return "DROP DATABASE " + mysqlQuoteIdent(request.Scope.Name("database")), nil
	case ddl.OperationRenameColumn:
		return "ALTER TABLE " + mysqlDDLRef(request) + " RENAME COLUMN " + mysqlQuoteIdent(request.Name) + " TO " + mysqlQuoteIdent(request.NewName), nil
	case ddl.OperationDropColumn:
		return "ALTER TABLE " + mysqlDDLRef(request) + " DROP COLUMN " + mysqlQuoteIdent(request.Name), nil
	case ddl.OperationDropIndex:
		return "ALTER TABLE " + mysqlDDLRef(request) + " DROP INDEX " + mysqlQuoteIdent(request.Name), nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func mysqlDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := ddl.CanonicalColumnType(column.DataType, mysqlDDLSpec.ColumnTypes)
		definition := mysqlQuoteIdent(column.Name) + " " + dataType
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
		if column.PrimaryKey {
			primary = append(primary, mysqlQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func mysqlDDLRef(request ddl.Request) string {
	return mysqlQuoteQualified(request.Ref.Scope.Name("database"), request.Ref.Name)
}
