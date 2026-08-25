package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*postgresDriver)(nil)

var postgresDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationDropScope,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes: []string{
		"bigint", "bigserial", "boolean", "bytea", "date", "double precision",
		"integer", "json", "jsonb", "numeric", "real", "smallint", "serial",
		"text", "time", "timestamp", "timestamp with time zone", "uuid", "varchar",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view", "materialized_view"},
	DroppableScopeKinds:      []string{"schema"},
	SupportsCascade:          true,
}

func (d *postgresDriver) DDLSpec() ddl.Spec {
	return postgresDDLSpec
}

func (d *postgresDriver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, postgresDDLSpec); err != nil {
		return err
	}
	statement, err := postgresDDLSQL(request)
	if err != nil {
		return err
	}
	// Every dynamic value is an identifier escaped by pgQuoteIdent or a data
	// type selected from postgresDDLSpec's closed allowlist.
	// codeql[go/sql-injection]
	if _, err := d.conn().ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("postgres: apply DDL: %w", err)
	}
	return nil
}

func postgresDDLSQL(request ddl.Request) (string, error) {
	cascade := ""
	if request.Cascade {
		cascade = " CASCADE"
	}
	switch request.Operation {
	case ddl.OperationCreateTable:
		return "CREATE TABLE " + postgresDDLQualified(request.Scope.Name("schema"), request.Name) + " (" + postgresDDLColumns(request.Columns) + ")", nil
	case ddl.OperationDropObject:
		verb := map[string]string{"table": "DROP TABLE", "view": "DROP VIEW", "materialized_view": "DROP MATERIALIZED VIEW"}[request.Ref.Kind]
		return verb + " " + postgresDDLQualified(request.Ref.Scope.Name("schema"), request.Ref.Name) + cascade, nil
	case ddl.OperationDropScope:
		return "DROP SCHEMA " + pgQuoteIdent(request.Scope.Name("schema")) + cascade, nil
	case ddl.OperationRenameColumn:
		return "ALTER TABLE " + postgresDDLRef(request) + " RENAME COLUMN " + pgQuoteIdent(request.Name) + " TO " + pgQuoteIdent(request.NewName), nil
	case ddl.OperationDropColumn:
		return "ALTER TABLE " + postgresDDLRef(request) + " DROP COLUMN " + pgQuoteIdent(request.Name) + cascade, nil
	case ddl.OperationDropIndex:
		return "DROP INDEX " + postgresDDLQualified(request.Ref.Scope.Name("schema"), request.Name) + cascade, nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func postgresDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := ddl.CanonicalColumnType(column.DataType, postgresDDLSpec.ColumnTypes)
		definition := pgQuoteIdent(column.Name) + " " + dataType
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
		if column.PrimaryKey {
			primary = append(primary, pgQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func postgresDDLRef(request ddl.Request) string {
	return postgresDDLQualified(request.Ref.Scope.Name("schema"), request.Ref.Name)
}

func postgresDDLQualified(schema, name string) string {
	return pgQuoteIdent(schema) + "." + pgQuoteIdent(name)
}
