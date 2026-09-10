package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
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
		ddl.OperationAddColumn,
		ddl.OperationAlterColumn,
		ddl.OperationCreateIndex,
	},
	ColumnTypes: []string{
		"bigint", "blob", "boolean", "date", "datetime", "double",
		"float", "int", "json", "mediumint", "smallint", "text", "time", "timestamp",
		"tinyint",
	},
	CreatableTableScopeKinds: []string{"database"},
	DroppableObjectKinds:     []string{"table", "view"},
	DroppableScopeKinds:      []string{"database"},
	SupportsColumnDefaults:   true,
	ParameterizedColumnTypes: []ddl.ParameterizedColumnType{
		{Name: "decimal", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 65}, {Name: "scale", Min: 0, Max: 30, Optional: true}}},
		{Name: "varchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 65535}}},
		{Name: "char", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 255}}},
		{Name: "varbinary", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 65535}}},
	},
}

func (d *Driver) DDLSpec() ddl.Spec {
	return mysqlDDLSpec
}

// ColumnDefault is one column's raw default as information_schema reports it,
// plus the session setting needed to render it back as SQL.
type ColumnDefault struct {
	// ColumnType and Extra are the column's information_schema.columns
	// column_type and extra values.
	ColumnType string
	Extra      string
	// Raw is the non-NULL column_default value.
	Raw string
	// NoBackslashEscapes reports whether the session's sql_mode includes
	// NO_BACKSLASH_ESCAPES, under which a backslash inside a string literal
	// is an ordinary character and must not be doubled.
	NoBackslashEscapes bool
}

// ColumnDefaultDecoder converts a ColumnDefault into an expression that can be
// re-emitted verbatim into DDL. MySQL and MariaDB disagree on the shape of the
// raw value (MySQL 8 strips the quotes from string literals and flags
// expression defaults in extra; MariaDB reports an already-valid expression in
// every case), so ApplyDDLWithDefaults takes the flavor's decoder rather than
// branching on the driver.
type ColumnDefaultDecoder interface {
	DecodeColumnDefault(ColumnDefault) string
	// SuppressGeneratedNullability reports whether a GENERATED column's
	// MODIFY COLUMN redeclaration must omit the NULL/NOT NULL clause
	// entirely. MySQL 8 accepts the clause on a GENERATED column (and
	// requires it to preserve NOT NULL across an edit); MariaDB rejects the
	// clause outright, in either form, on a GENERATED column.
	SuppressGeneratedNullability() bool
}

// DefaultDecoder implements ColumnDefaultDecoder for MySQL 8's
// information_schema semantics.
type DefaultDecoder struct{}

func (DefaultDecoder) DecodeColumnDefault(def ColumnDefault) string {
	return mysqlRequoteDefault(def)
}

func (DefaultDecoder) SuppressGeneratedNullability() bool { return false }

func (d *Driver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	return d.ApplyDDLWithDefaults(ctx, request, DefaultDecoder{})
}

// ApplyDDLWithDefaults is ApplyDDL parameterized by the flavor-specific
// reading of information_schema column defaults. Engines that embed Driver
// but whose server reports defaults differently override ApplyDDL to call
// this with their own ColumnDefaultDecoder; Go method promotion has no
// virtual dispatch, so the embedded ApplyDDL cannot reach the embedder.
func (d *Driver) ApplyDDLWithDefaults(ctx context.Context, request ddl.Request, decoder ColumnDefaultDecoder) error {
	if err := ddl.Validate(request, mysqlDDLSpec); err != nil {
		return err
	}
	if err := validateMySQLDefaults(request); err != nil {
		return err
	}
	if request.Operation == ddl.OperationAlterColumn {
		current, err := currentMySQLColumn(ctx, d.db, *request.Ref, request.Name, decoder)
		if err != nil {
			return err
		}
		statement := mysqlAlterColumnSQL(request, current)
		// codeql[go/sql-injection]
		if _, err := d.conn().ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("mysql: apply DDL: %w", err)
		}
		return nil
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
	case ddl.OperationAddColumn:
		return "ALTER TABLE " + mysqlDDLRef(request) + " ADD COLUMN " + mysqlDDLColumn(*request.Column), nil
	case ddl.OperationCreateIndex:
		prefix := "ADD "
		if request.Unique {
			prefix += "UNIQUE "
		}
		columns := make([]string, len(request.IndexColumns))
		for i, column := range request.IndexColumns {
			columns[i] = mysqlQuoteIdent(column.Name)
			if column.Descending {
				columns[i] += " DESC"
			}
		}
		return "ALTER TABLE " + mysqlDDLRef(request) + " " + prefix + "INDEX " + mysqlQuoteIdent(request.Name) + " (" + strings.Join(columns, ", ") + ")", nil
	default:
		return "", fmt.Errorf("%w: operation %q", ddl.ErrUnsupported, request.Operation)
	}
}

func mysqlDDLColumns(columns []ddl.ColumnDefinition) string {
	definitions := make([]string, 0, len(columns)+1)
	primary := make([]string, 0, len(columns))
	for _, column := range columns {
		definitions = append(definitions, mysqlDDLColumn(column))
		if column.PrimaryKey {
			primary = append(primary, mysqlQuoteIdent(column.Name))
		}
	}
	if len(primary) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+strings.Join(primary, ", ")+")")
	}
	return strings.Join(definitions, ", ")
}

func mysqlDDLColumn(column ddl.ColumnDefinition) string {
	dataType, ok := ddl.CanonicalColumnType(column.DataType, mysqlDDLSpec.ColumnTypes)
	if !ok {
		dataType, _ = mysqlDDLSpec.CanonicalColumnType(column.DataType)
	}
	definition := mysqlQuoteIdent(column.Name) + " " + dataType
	if !column.Nullable || column.PrimaryKey {
		definition += " NOT NULL"
	} else {
		definition += " NULL"
	}
	if column.Default != nil {
		definition += " DEFAULT " + strings.TrimSpace(*column.Default)
	}
	return definition
}

func mysqlDDLRef(request ddl.Request) string {
	return mysqlQuoteQualified(request.Ref.Scope.Name("database"), request.Ref.Name)
}

func mysqlDDLQualified(database, name string) string {
	return mysqlQuoteQualified(database, name)
}

// mysqlColumnState is a column as it currently exists on the server: the
// portable definition plus the MySQL-only attributes that MODIFY COLUMN drops
// unless they are re-emitted. It is deliberately internal to this package
// rather than extra fields on ddl.ColumnDefinition: these values only ever
// originate from information_schema, and hanging them off the request struct
// the API decodes from client JSON would let unvalidated caller-supplied
// comment/collation text reach generated DDL through create_table/add_column.
type mysqlColumnState struct {
	ddl.ColumnDefinition
	// Comment is information_schema.columns.column_comment, empty when unset.
	Comment string
	// Collation is information_schema.columns.collation_name, empty for
	// non-character columns.
	Collation string
	// AutoIncrement and OnUpdate are parsed out of
	// information_schema.columns.extra, which packs both into one free-text
	// field (e.g. "DEFAULT_GENERATED on update CURRENT_TIMESTAMP").
	AutoIncrement bool
	OnUpdate      string
	// Generated, GenerationExpression, and GenerationStored describe a
	// GENERATED ALWAYS AS (...) column. generation_expression is not
	// consistently self-parenthesized (MySQL wraps a binary expression like
	// `price` * 2 in parens, MariaDB and MySQL's own function-call
	// expressions do not), so the caller must always add its own wrapping
	// parens rather than assume the stored text already has them. A
	// generated column never carries a Default, AutoIncrement, or OnUpdate,
	// since MySQL/MariaDB reject those clauses together with GENERATED.
	Generated            bool
	GenerationExpression string
	GenerationStored     bool
	// SuppressGeneratedNullability mirrors the decoder's
	// ColumnDefaultDecoder.SuppressGeneratedNullability, carried onto the
	// column state so mysqlModifyColumnDefinition can consult it without
	// its own flavor parameter.
	SuppressGeneratedNullability bool
	// NoBackslashEscapes mirrors ColumnDefault.NoBackslashEscapes for the
	// session the column was read on, so re-emitted string literals (the
	// COMMENT clause) are escaped the way that session will read them back.
	NoBackslashEscapes bool
}

// currentMySQLColumn reads back a column's live definition so
// mysqlAlterColumnSQL can redeclare it unchanged when the request omits it —
// MODIFY COLUMN has no partial-attribute form, unlike Postgres/Oracle's
// clause-per-attribute ALTER COLUMN, so every attribute not read here is
// silently dropped by an edit that only changes the type.
func currentMySQLColumn(ctx context.Context, db *sql.DB, ref metadata.ObjectRef, name string, decoder ColumnDefaultDecoder) (mysqlColumnState, error) {
	var dataType, nullable, extra, comment, sqlMode string
	var def, collation, generationExpression sql.NullString
	err := db.QueryRowContext(ctx, `
SELECT column_type, is_nullable, column_default, extra, column_comment, collation_name, generation_expression, @@session.sql_mode
FROM information_schema.columns
WHERE table_schema = ? AND table_name = ? AND column_name = ?`,
		ref.Scope.Name("database"), ref.Name, name).Scan(&dataType, &nullable, &def, &extra, &comment, &collation, &generationExpression, &sqlMode)
	if err != nil {
		return mysqlColumnState{}, fmt.Errorf("mysql: current column: %w", err)
	}
	autoIncrement, onUpdate := mysqlParseColumnExtra(extra)
	noBackslashEscapes := strings.Contains(strings.ToUpper(sqlMode), "NO_BACKSLASH_ESCAPES")
	column := mysqlColumnState{
		ColumnDefinition:             ddl.ColumnDefinition{Name: name, DataType: dataType, Nullable: nullable == "YES"},
		Comment:                      comment,
		Collation:                    collation.String,
		AutoIncrement:                autoIncrement,
		OnUpdate:                     onUpdate,
		Generated:                    generationExpression.String != "",
		GenerationExpression:         generationExpression.String,
		GenerationStored:             strings.Contains(strings.ToUpper(extra), "STORED GENERATED"),
		SuppressGeneratedNullability: decoder.SuppressGeneratedNullability(),
		NoBackslashEscapes:           noBackslashEscapes,
	}
	if column.Generated {
		return column, nil
	}
	if def.Valid {
		v := decoder.DecodeColumnDefault(ColumnDefault{
			ColumnType:         dataType,
			Extra:              extra,
			Raw:                def.String,
			NoBackslashEscapes: noBackslashEscapes,
		})
		column.Default = &v
	}
	return column, nil
}

// mysqlParseColumnExtra splits information_schema.columns.extra into the two
// attributes a column redeclaration must carry over. extra is free text that
// concatenates several markers (e.g. "DEFAULT_GENERATED on update
// CURRENT_TIMESTAMP"), and MariaDB lowercases and parenthesizes the function
// ("on update current_timestamp()"), so matching is case-insensitive and the
// ON UPDATE expression is taken verbatim from the server's own spelling.
func mysqlParseColumnExtra(extra string) (autoIncrement bool, onUpdate string) {
	lower := strings.ToLower(extra)
	autoIncrement = strings.Contains(lower, "auto_increment")
	const marker = "on update "
	if idx := strings.Index(lower, marker); idx >= 0 {
		onUpdate = strings.TrimSpace(extra[idx+len(marker):])
	}
	return autoIncrement, onUpdate
}

// mysqlCollatableTypes lists the base column types that accept a COLLATE
// clause. A column's collation is only re-emitted when the redeclared type is
// still one of these, so changing e.g. varchar to int does not produce a
// COLLATE clause MySQL rejects.
var mysqlCollatableTypes = map[string]bool{
	"char": true, "varchar": true,
	"text": true, "tinytext": true, "mediumtext": true, "longtext": true,
	"enum": true, "set": true,
}

// mysqlTemporalDefaultTypes lists the base column types that accept
// ON UPDATE CURRENT_TIMESTAMP, so the clause is dropped rather than re-emitted
// when an edit changes the column away from a timestamp/datetime.
var mysqlTemporalDefaultTypes = map[string]bool{
	"timestamp": true, "datetime": true,
}

// mysqlDefaultLiteralTypes lists MySQL base column types whose
// information_schema.columns.column_default value MySQL reports unquoted
// (e.g. a VARCHAR column with DEFAULT 'unnamed' reads back as the bare text
// unnamed), so a preserved default of these types must be re-quoted as a SQL
// string literal before mysqlDDLColumn re-emits it verbatim into DDL.
var mysqlDefaultLiteralTypes = map[string]bool{
	"char": true, "varchar": true,
	"text": true, "tinytext": true, "mediumtext": true, "longtext": true,
	"binary": true, "varbinary": true,
	"blob": true, "tinyblob": true, "mediumblob": true, "longblob": true,
	"enum": true, "set": true,
	"date": true, "time": true, "datetime": true, "timestamp": true, "year": true,
}

// mysqlBaseType strips the length/precision and any attribute suffix from an
// information_schema column_type (e.g. "varchar(50)" or "int unsigned"),
// yielding the lowercase base type name used for family lookups.
func mysqlBaseType(columnType string) string {
	base := columnType
	if idx := strings.IndexAny(base, "( "); idx >= 0 {
		base = base[:idx]
	}
	return strings.ToLower(base)
}

// mysqlRequoteDefault re-quotes a raw column_default value read back from
// information_schema.columns when its base column type is one MySQL reports
// unquoted, so currentMySQLColumn's preserved value survives being re-emitted
// verbatim by mysqlDDLColumn. extra containing DEFAULT_GENERATED marks an
// expression default (e.g. CURRENT_TIMESTAMP, or "(expr)") rather than a
// literal, so it is left untouched. Types whose defaults MySQL already reports
// as a valid literal (notably bit, which reads back as b'1') are likewise left
// alone: re-quoting those would corrupt them.
func mysqlRequoteDefault(def ColumnDefault) string {
	if strings.Contains(strings.ToUpper(def.Extra), "DEFAULT_GENERATED") {
		return def.Raw
	}
	if !mysqlDefaultLiteralTypes[mysqlBaseType(def.ColumnType)] {
		return def.Raw
	}
	return mysqlQuoteString(def.Raw, def.NoBackslashEscapes)
}

// mysqlQuoteString renders value as a SQL string literal. An embedded single
// quote is always doubled, which is valid under every sql_mode. A backslash is
// only doubled when the session interprets backslash as an escape character;
// under NO_BACKSLASH_ESCAPES it is an ordinary character and doubling it would
// store two.
func mysqlQuoteString(value string, noBackslashEscapes bool) string {
	if !noBackslashEscapes {
		value = strings.ReplaceAll(value, `\`, `\\`)
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// mysqlAlterColumnSQL redeclares column via MODIFY COLUMN, applying
// request.Changes over current and keeping every unspecified attribute as-is.
func mysqlAlterColumnSQL(request ddl.Request, current mysqlColumnState) string {
	merged := current
	if request.Changes.DataType != nil {
		dataType, ok := ddl.CanonicalColumnType(*request.Changes.DataType, mysqlDDLSpec.ColumnTypes)
		if !ok {
			dataType, _ = mysqlDDLSpec.CanonicalColumnType(*request.Changes.DataType)
		}
		merged.DataType = dataType
	}
	if request.Changes.Nullable != nil {
		merged.Nullable = *request.Changes.Nullable
	}
	if request.Changes.Default != nil {
		if strings.TrimSpace(*request.Changes.Default) == "" {
			merged.Default = nil
		} else {
			v := strings.TrimSpace(*request.Changes.Default)
			merged.Default = &v
		}
	}
	return "ALTER TABLE " + mysqlDDLQualified(request.Ref.Scope.Name("database"), request.Ref.Name) +
		" MODIFY COLUMN " + mysqlModifyColumnDefinition(merged)
}

// mysqlModifyColumnDefinition renders a full column redeclaration for MODIFY
// COLUMN. Beyond the portable type/nullability/default it re-emits the
// MySQL-only attributes carried on mysqlColumnState, because MODIFY COLUMN
// replaces the whole definition: anything omitted here is dropped from the
// live column even when the edit never mentioned it.
func mysqlModifyColumnDefinition(column mysqlColumnState) string {
	base := mysqlBaseType(column.DataType)
	definition := mysqlQuoteIdent(column.Name) + " " + column.DataType
	if column.Collation != "" && mysqlCollatableTypes[base] {
		definition += " COLLATE " + column.Collation
	}
	if column.Generated {
		definition += " GENERATED ALWAYS AS (" + column.GenerationExpression + ")"
		if column.GenerationStored {
			definition += " STORED"
		} else {
			definition += " VIRTUAL"
		}
	}
	if !column.Generated || !column.SuppressGeneratedNullability {
		if !column.Nullable || column.PrimaryKey {
			definition += " NOT NULL"
		} else {
			definition += " NULL"
		}
	}
	if !column.Generated {
		if column.Default != nil {
			definition += " DEFAULT " + strings.TrimSpace(*column.Default)
		}
		if column.OnUpdate != "" && mysqlTemporalDefaultTypes[base] {
			definition += " ON UPDATE " + column.OnUpdate
		}
		if column.AutoIncrement {
			definition += " AUTO_INCREMENT"
		}
	}
	if column.Comment != "" {
		definition += " COMMENT " + mysqlQuoteString(column.Comment, column.NoBackslashEscapes)
	}
	return definition
}
