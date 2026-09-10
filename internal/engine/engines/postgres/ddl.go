package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*Driver)(nil)

var postgresDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationDropScope,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
		ddl.OperationAddColumn,
		ddl.OperationAlterColumn,
		ddl.OperationCreateIndex,
	},
	// Every built-in type Postgres ships without an extension. Extension
	// types (pgvector, PostGIS, citext, hstore, ...) fall through to
	// AllowCustomColumnTypes below rather than being enumerated here.
	ColumnTypes: []string{
		"bigint", "bigserial", "bit", "bit varying", "boolean", "box", "bytea",
		"char", "cidr", "circle", "date", "datemultirange", "daterange",
		"decimal", "double precision", "inet", "int4multirange", "int4range",
		"int8multirange", "int8range", "integer", "interval", "json", "jsonb",
		"line", "lseg", "macaddr", "macaddr8", "money", "numeric",
		"nummultirange", "numrange", "oid", "path", "pg_lsn", "point",
		"polygon", "real", "serial", "smallint", "smallserial", "text",
		"time", "time with time zone", "timestamp", "timestamp with time zone",
		"tsmultirange", "tsquery", "tsrange", "tstzmultirange", "tstzrange",
		"tsvector", "uuid", "varchar", "xml",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view", "materialized_view"},
	DroppableScopeKinds:      []string{"schema"},
	SupportsCascade:          true,
	SupportsColumnDefaults:   true,
	// Extensions (pgvector, PostGIS, citext, hstore, ...) add types outside
	// this base grammar; let the database validate anything syntactically safe.
	AllowCustomColumnTypes: true,
	ParameterizedColumnTypes: []ddl.ParameterizedColumnType{
		{Name: "numeric", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 1000}, {Name: "scale", Min: 0, Max: 1000, Optional: true}}},
		{Name: "decimal", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 1000}, {Name: "scale", Min: 0, Max: 1000, Optional: true}}},
		{Name: "varchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "char", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "bit", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "bit varying", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "time", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "time", Suffix: "with time zone", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "timestamp", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "timestamp", Suffix: "with time zone", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "interval", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
	},
}

func (d *Driver) DDLSpec() ddl.Spec {
	return postgresDDLSpec
}

func (d *Driver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	if err := ddl.Validate(request, postgresDDLSpec); err != nil {
		return err
	}
	if err := validatePostgresDefaults(request); err != nil {
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
	case ddl.OperationAddColumn:
		return "ALTER TABLE " + postgresDDLRef(request) + " ADD COLUMN " + postgresDDLColumn(*request.Column), nil
	case ddl.OperationAlterColumn:
		var clauses []string
		if request.Changes.DataType != nil {
			dataType, _ := postgresDDLSpec.CanonicalColumnType(*request.Changes.DataType)
			clauses = append(clauses, "ALTER COLUMN "+pgQuoteIdent(request.Name)+" TYPE "+dataType)
		}
		if request.Changes.Default != nil {
			if strings.TrimSpace(*request.Changes.Default) == "" {
				clauses = append(clauses, "ALTER COLUMN "+pgQuoteIdent(request.Name)+" DROP DEFAULT")
			} else {
				clauses = append(clauses, "ALTER COLUMN "+pgQuoteIdent(request.Name)+" SET DEFAULT ("+strings.TrimSpace(*request.Changes.Default)+")")
			}
		}
		if request.Changes.Nullable != nil {
			verb := "SET NOT NULL"
			if *request.Changes.Nullable {
				verb = "DROP NOT NULL"
			}
			clauses = append(clauses, "ALTER COLUMN "+pgQuoteIdent(request.Name)+" "+verb)
		}
		return "ALTER TABLE " + postgresDDLRef(request) + " " + strings.Join(clauses, ", "), nil
	case ddl.OperationCreateIndex:
		prefix := "CREATE "
		if request.Unique {
			prefix += "UNIQUE "
		}
		columns := make([]string, len(request.IndexColumns))
		for i, column := range request.IndexColumns {
			columns[i] = pgQuoteIdent(column.Name)
			if column.Descending {
				columns[i] += " DESC"
			}
		}
		return prefix + "INDEX " + pgQuoteIdent(request.Name) + " ON " + postgresDDLRef(request) + " (" + strings.Join(columns, ", ") + ")", nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func postgresDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, postgresDDLColumn(column))
		if column.PrimaryKey {
			primary = append(primary, pgQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func postgresDDLColumn(column ddl.ColumnDefinition) string {
	dataType, _ := ddl.CanonicalColumnType(column.DataType, postgresDDLSpec.ColumnTypes)
	if dataType == "" {
		dataType, _ = postgresDDLSpec.CanonicalColumnType(column.DataType)
	}
	definition := pgQuoteIdent(column.Name) + " " + dataType
	if column.Default != nil {
		definition += " DEFAULT (" + strings.TrimSpace(*column.Default) + ")"
	}
	if !column.Nullable || column.PrimaryKey {
		definition += " NOT NULL"
	}
	return definition
}

func postgresDDLRef(request ddl.Request) string {
	return postgresDDLQualified(request.Ref.Scope.Name("schema"), request.Ref.Name)
}

func postgresDDLQualified(schema, name string) string {
	return pgQuoteIdent(schema) + "." + pgQuoteIdent(name)
}
