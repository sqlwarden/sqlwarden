package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/build"
)

// CatalogTables enumerates every table and view in the connection's current
// database's schemas, invoking add once per object with its schema, name,
// and resolved kind ("table" or "view"). sys.objects/sys.schemas (not
// information_schema) are used so later tasks can join the same object
// identity against sys.identity_columns and sys.computed_columns without a
// second lookup.
//
// There is no database parameter: SQL Server has no parameterized USE
// statement, and sys.* views are always scoped to the connection's *current*
// database — there is no cross-database sys.tables query. Connect already
// selects the target database via msdsn.Config.Database, so this query never
// needs to switch databases itself.
func CatalogTables(ctx context.Context, db *sql.DB, add func(schema, name, kind string)) error {
	const stmt = `
SELECT s.name AS schema_name, o.name AS object_name, o.type
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE o.type IN ('U', 'V') AND o.is_ms_shipped = 0
ORDER BY s.name, o.name`
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return fmt.Errorf("sqlserver: catalog tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name, objType string
		if err := rows.Scan(&schema, &name, &objType); err != nil {
			return fmt.Errorf("sqlserver: catalog tables scan: %w", err)
		}
		kind := "table"
		if strings.TrimSpace(objType) == "V" {
			kind = "view"
		}
		add(schema, name, kind)
	}
	return rows.Err()
}

// sqlServerValuesFilter builds a T-SQL VALUES derived-table filter, since
// SQL Server has no row-value IN clause ("WHERE (a,b) IN ((@p1,@p2),...)" is
// not valid T-SQL, unlike MySQL/Postgres). Callers JOIN the result against
// this derived table instead of using it in a WHERE ... IN.
func sqlServerValuesFilter(refs []metadata.ObjectRef) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	for i, ref := range refs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("(@p%d, @p%d)", len(args)+1, len(args)+2))
		args = append(args, ref.Scope.Name("schema"), ref.Name)
	}
	return sb.String(), args
}

// RelationalObjects fetches full column/PK/FK/index detail for tables and
// views named in refs.
func RelationalObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	refByName := make(map[string]metadata.ObjectRef, len(refs))
	for _, ref := range refs {
		refByName[ref.Scope.Name("schema")+"\x00"+ref.Name] = ref
	}
	refFor := func(schema, name string) metadata.ObjectRef {
		return refByName[schema+"\x00"+name]
	}

	b := build.NewRelational()
	for _, ref := range refs {
		b.Ensure(ref)
	}

	values, args := sqlServerValuesFilter(refs)

	colQ := `
SELECT s.name, o.name, c.name, ty.name, c.max_length, c.precision, c.scale, c.is_nullable,
       c.column_id, dc.definition, c.is_identity, ic.seed_value, ic.increment_value, cc.definition
FROM sys.columns c
JOIN sys.objects o ON o.object_id = c.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN (VALUES ` + values + `) AS f(schema_name, object_name) ON f.schema_name = s.name AND f.object_name = o.name
JOIN sys.types ty ON ty.user_type_id = c.user_type_id
LEFT JOIN sys.default_constraints dc ON dc.object_id = c.default_object_id
LEFT JOIN sys.identity_columns ic ON ic.object_id = c.object_id AND ic.column_id = c.column_id
LEFT JOIN sys.computed_columns cc ON cc.object_id = c.object_id AND cc.column_id = c.column_id
ORDER BY s.name, o.name, c.column_id`
	crows, err := db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: object columns: %w", err)
	}
	for crows.Next() {
		var schema, table, col, typeName string
		var maxLength int
		var precision, scale int
		var nullable bool
		var ordinal int
		var def, computedDef sql.NullString
		var isIdentity bool
		var seed, increment sql.NullInt64
		if err := crows.Scan(&schema, &table, &col, &typeName, &maxLength, &precision, &scale,
			&nullable, &ordinal, &def, &isIdentity, &seed, &increment, &computedDef); err != nil {
			crows.Close()
			return nil, fmt.Errorf("sqlserver: object columns scan: %w", err)
		}
		c := metadata.Column{
			Name:     col,
			DataType: sqlServerFormatType(typeName, maxLength, precision, scale),
			Nullable: nullable,
			Ordinal:  ordinal,
		}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		if isIdentity && seed.Valid && increment.Valid {
			setColumnAttr(&c, "identity", fmt.Sprintf("%d,%d", seed.Int64, increment.Int64))
		}
		if computedDef.Valid {
			setColumnAttr(&c, "computed", computedDef.String)
		}
		b.AddColumn(refFor(schema, table), c)
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return nil, fmt.Errorf("sqlserver: object columns rows: %w", err)
	}
	crows.Close()

	pkQ := `
SELECT s.name, o.name, col.name
FROM sys.key_constraints kc
JOIN sys.objects o ON o.object_id = kc.parent_object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN sys.index_columns ic ON ic.object_id = kc.parent_object_id AND ic.index_id = kc.unique_index_id
JOIN sys.columns col ON col.object_id = ic.object_id AND col.column_id = ic.column_id
JOIN (VALUES ` + values + `) AS f(schema_name, object_name) ON f.schema_name = s.name AND f.object_name = o.name
WHERE kc.type = 'PK'
ORDER BY s.name, o.name, ic.key_ordinal`
	prows, err := db.QueryContext(ctx, pkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: object pk: %w", err)
	}
	for prows.Next() {
		var schema, table, col string
		if err := prows.Scan(&schema, &table, &col); err != nil {
			prows.Close()
			return nil, fmt.Errorf("sqlserver: object pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(refFor(schema, table), col)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, fmt.Errorf("sqlserver: object pk rows: %w", err)
	}
	prows.Close()

	fkQ := `
SELECT s.name, o.name, fk.name, col.name, rs.name, ro.name, rcol.name
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
JOIN sys.objects o ON o.object_id = fk.parent_object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN sys.columns col ON col.object_id = fkc.parent_object_id AND col.column_id = fkc.parent_column_id
JOIN sys.objects ro ON ro.object_id = fk.referenced_object_id
JOIN sys.schemas rs ON rs.schema_id = ro.schema_id
JOIN sys.columns rcol ON rcol.object_id = fkc.referenced_object_id AND rcol.column_id = fkc.referenced_column_id
JOIN (VALUES ` + values + `) AS f(schema_name, object_name) ON f.schema_name = s.name AND f.object_name = o.name
ORDER BY s.name, o.name, fk.name, fkc.constraint_column_id`
	frows, err := db.QueryContext(ctx, fkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: object fk: %w", err)
	}
	for frows.Next() {
		var schema, table, name, col, refSchema, refTable, refCol string
		if err := frows.Scan(&schema, &table, &name, &col, &refSchema, &refTable, &refCol); err != nil {
			frows.Close()
			return nil, fmt.Errorf("sqlserver: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(refFor(schema, table), name, col,
			metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: refSchema}), Kind: "table", Name: refTable}, refCol)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("sqlserver: object fk rows: %w", err)
	}
	frows.Close()

	idxQ := `
SELECT s.name, o.name, i.name, i.is_unique, col.name, ic.key_ordinal
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
JOIN sys.columns col ON col.object_id = ic.object_id AND col.column_id = ic.column_id
JOIN (VALUES ` + values + `) AS f(schema_name, object_name) ON f.schema_name = s.name AND f.object_name = o.name
WHERE i.is_primary_key = 0 AND i.name IS NOT NULL AND ic.is_included_column = 0
ORDER BY s.name, o.name, i.name, ic.key_ordinal`
	irows, err := db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: object indexes: %w", err)
	}
	type idxKey struct{ schema, table, name string }
	indexes := map[idxKey]*metadata.SecondaryIndex{}
	var indexOrder []idxKey
	for irows.Next() {
		var schema, table, name, col string
		var unique bool
		var ordinal int
		if err := irows.Scan(&schema, &table, &name, &unique, &col, &ordinal); err != nil {
			irows.Close()
			return nil, fmt.Errorf("sqlserver: object index scan: %w", err)
		}
		key := idxKey{schema: schema, table: table, name: name}
		ix, ok := indexes[key]
		if !ok {
			ix = &metadata.SecondaryIndex{Name: name, Unique: unique}
			indexes[key] = ix
			indexOrder = append(indexOrder, key)
		}
		ix.Columns = append(ix.Columns, col)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, fmt.Errorf("sqlserver: object index rows: %w", err)
	}
	irows.Close()
	for _, key := range indexOrder {
		b.AddIndex(refFor(key.schema, key.table), *indexes[key])
	}

	return b.Build(), nil
}

// sqlServerFormatType renders a sys.types name plus length/precision/scale
// into a human-readable T-SQL type string (e.g. "nvarchar(100)",
// "decimal(10,2)"). maxLength of -1 means MAX (nvarchar(max), varbinary(max)).
func sqlServerFormatType(name string, maxLength, precision, scale int) string {
	switch name {
	case "nvarchar", "nchar":
		if maxLength == -1 {
			return name + "(max)"
		}
		return fmt.Sprintf("%s(%d)", name, maxLength/2)
	case "varchar", "char", "varbinary", "binary":
		if maxLength == -1 {
			return name + "(max)"
		}
		return fmt.Sprintf("%s(%d)", name, maxLength)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", name, precision, scale)
	default:
		return name
	}
}

func setColumnAttr(c *metadata.Column, key, value string) {
	if value == "" {
		return
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	c.Attributes[key] = value
}

func sqlServerQuoteIdent(s string) string {
	return "[" + strings.ReplaceAll(s, "]", "]]") + "]"
}

func sqlServerQuoteQualified(schema, name string) string {
	return sqlServerQuoteIdent(schema) + "." + sqlServerQuoteIdent(name)
}
