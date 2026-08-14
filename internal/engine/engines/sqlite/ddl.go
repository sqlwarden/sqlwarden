package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*sqliteDriver)(nil)

var sqliteDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes:              []string{"blob", "integer", "numeric", "real", "text"},
	CreatableTableScopeKinds: []string{"database"},
	DroppableObjectKinds:     []string{"table", "view"},
}

func (d *sqliteDriver) DDLSpec() ddl.Spec {
	return sqliteDDLSpec
}

func (d *sqliteDriver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, sqliteDDLSpec); err != nil {
		return err
	}
	statement, err := sqliteDDLSQL(request)
	if err != nil {
		return err
	}
	// Every dynamic value is escaped by sqliteQuoteIdent or selected from the
	// closed data-type allowlist above.
	// codeql[go/sql-injection]
	if _, err := d.db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("sqlite: apply DDL: %w", err)
	}
	return nil
}

func sqliteDDLSQL(request ddl.Request) (string, error) {
	switch request.Operation {
	case ddl.OperationCreateTable:
		return "CREATE TABLE " + sqliteDDLQualified(request.Scope.Name("database"), request.Name) + " (" + sqliteDDLColumns(request.Columns) + ")", nil
	case ddl.OperationDropObject:
		verb := map[string]string{"table": "DROP TABLE", "view": "DROP VIEW"}[request.Ref.Kind]
		return verb + " " + sqliteDDLQualified(request.Ref.Scope.Name("database"), request.Ref.Name), nil
	case ddl.OperationRenameColumn:
		return "ALTER TABLE " + sqliteDDLRef(request) + " RENAME COLUMN " + sqliteQuoteIdent(request.Name) + " TO " + sqliteQuoteIdent(request.NewName), nil
	case ddl.OperationDropColumn:
		return "ALTER TABLE " + sqliteDDLRef(request) + " DROP COLUMN " + sqliteQuoteIdent(request.Name), nil
	case ddl.OperationDropIndex:
		return "DROP INDEX " + sqliteDDLQualified(request.Ref.Scope.Name("database"), request.Name), nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func sqliteDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := ddl.CanonicalColumnType(column.DataType, sqliteDDLSpec.ColumnTypes)
		definition := sqliteQuoteIdent(column.Name) + " " + dataType
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
		if column.PrimaryKey {
			primary = append(primary, sqliteQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func sqliteDDLRef(request ddl.Request) string {
	return sqliteDDLQualified(request.Ref.Scope.Name("database"), request.Ref.Name)
}

func sqliteDDLQualified(database, name string) string {
	return sqliteQuoteIdent(database) + "." + sqliteQuoteIdent(name)
}
