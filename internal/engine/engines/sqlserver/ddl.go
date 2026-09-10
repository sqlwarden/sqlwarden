package sqlserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*Driver)(nil)

var sqlServerDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationDropScope,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes: []string{
		"bigint", "binary(255)", "bit", "char(255)", "date", "datetime",
		"datetime2", "datetimeoffset", "decimal(10,2)", "float", "geography",
		"geometry", "hierarchyid", "image", "int", "money", "nchar(255)",
		"ntext", "numeric(18,0)", "nvarchar(255)", "nvarchar(max)", "real",
		"rowversion", "smalldatetime", "smallint", "smallmoney", "sql_variant",
		"text", "time", "tinyint", "uniqueidentifier", "varbinary(255)",
		"varbinary(max)", "varchar(255)", "varchar(max)", "xml",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view"},
	DroppableScopeKinds:      []string{"schema"},
	ParameterizedColumnTypes: []ddl.ParameterizedColumnType{
		{Name: "decimal", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 38}, {Name: "scale", Min: 0, Max: 38, Optional: true}}},
		{Name: "numeric", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 38}, {Name: "scale", Min: 0, Max: 38, Optional: true}}},
		{Name: "varchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 8000}}},
		{Name: "char", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 8000}}},
		{Name: "nvarchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 4000}}},
		{Name: "nchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 4000}}},
		{Name: "varbinary", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 8000}}},
		{Name: "binary", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 8000}}},
		{Name: "time", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 7}}},
		{Name: "datetime2", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 7}}},
		{Name: "datetimeoffset", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 7}}},
	},
}

func (d *Driver) DDLSpec() ddl.Spec {
	return sqlServerDDLSpec
}

func (d *Driver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, sqlServerDDLSpec); err != nil {
		return err
	}
	statement, err := sqlServerDDLSQL(request)
	if err != nil {
		return err
	}
	// Every dynamic value is escaped by sqlServerQuoteIdent, sqlServerSQLLiteralEscape,
	// or selected from the closed data-type allowlist above.
	// codeql[go/sql-injection]
	if _, err := d.conn().ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("sqlserver: apply DDL: %w", err)
	}
	return nil
}

func sqlServerDDLSQL(request ddl.Request) (string, error) {
	switch request.Operation {
	case ddl.OperationCreateTable:
		return "CREATE TABLE " + sqlServerQuoteQualified(request.Scope.Name("schema"), request.Name) + " (" + sqlServerDDLColumns(request.Columns) + ")", nil
	case ddl.OperationDropObject:
		verb := map[string]string{"table": "DROP TABLE", "view": "DROP VIEW"}[request.Ref.Kind]
		return verb + " " + sqlServerQuoteQualified(request.Ref.Scope.Name("schema"), request.Ref.Name), nil
	case ddl.OperationDropScope:
		return "DROP SCHEMA " + sqlServerQuoteIdent(request.Scope.Name("schema")), nil
	case ddl.OperationRenameColumn:
		// sp_rename takes its target and new name as string literals, not
		// identifiers. ddl.ValidateIdentifier (run by ddl.Validate above) only
		// rejects empty/whitespace/NUL names, not quote characters, so quotes
		// are escaped explicitly here rather than assumed safe.
		return fmt.Sprintf("EXEC sp_rename '%s.%s', '%s', 'COLUMN'",
			sqlServerSQLLiteralEscape(sqlServerDDLRefLiteral(request)),
			sqlServerSQLLiteralEscape(request.Name),
			sqlServerSQLLiteralEscape(request.NewName)), nil
	case ddl.OperationDropColumn:
		return "ALTER TABLE " + sqlServerDDLRef(request) + " DROP COLUMN " + sqlServerQuoteIdent(request.Name), nil
	case ddl.OperationDropIndex:
		return "DROP INDEX " + sqlServerQuoteIdent(request.Name) + " ON " + sqlServerDDLRef(request), nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func sqlServerDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		dataType, _ := sqlServerDDLSpec.CanonicalColumnType(column.DataType)
		definition := sqlServerQuoteIdent(column.Name) + " " + dataType
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
		if column.PrimaryKey {
			primary = append(primary, sqlServerQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func sqlServerDDLRef(request ddl.Request) string {
	return sqlServerQuoteQualified(request.Ref.Scope.Name("schema"), request.Ref.Name)
}

// sqlServerDDLRefLiteral renders schema.table without brackets, for
// sp_rename's string-literal object-name argument (@objname), which SQL
// Server resolves by name lookup, not as a T-SQL identifier expression.
func sqlServerDDLRefLiteral(request ddl.Request) string {
	return request.Ref.Scope.Name("schema") + "." + request.Ref.Name
}

// sqlServerSQLLiteralEscape escapes a value for embedding inside a single-
// quoted T-SQL string literal. sp_rename's @objname/@newname arguments are
// string literals resolved by name lookup, not T-SQL identifier expressions,
// so bracket-quoting does not apply here; only literal escaping does.
func sqlServerSQLLiteralEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
