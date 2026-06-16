package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/driver"
	build "github.com/sqlwarden/internal/driver/internal/build"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	driver.Register("postgres", func() driver.Driver { return &postgresDriver{} })
}

type postgresDriver struct {
	db *sql.DB
}

func (d *postgresDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	d.db = db
	return nil
}

func (d *postgresDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *postgresDriver) Close() error {
	return d.db.Close()
}

func (d *postgresDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *postgresDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: execute: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *postgresDriver) Dialect() driver.Dialect {
	return driver.DialectPostgres
}

func (d *postgresDriver) Introspect(ctx context.Context, opts schema.IntrospectOptions) (*schema.Schema, error) {
	const q = `
WITH cols AS (
	-- udt_name is the raw pg_catalog type (varchar, timestamptz, int8, bool, …);
	-- information_schema.data_type returns verbose SQL-standard names instead
	-- (e.g. "character varying", "timestamp without time zone").
	SELECT table_schema, table_name, column_name, udt_name, is_nullable,
	       column_default, ordinal_position
	FROM information_schema.columns
	WHERE table_catalog = current_database()
	  AND table_schema NOT IN ('pg_catalog', 'information_schema')
)
SELECT t.table_schema, t.table_name, t.table_type,
       c.column_name, c.udt_name, c.is_nullable, c.column_default, c.ordinal_position
FROM information_schema.tables t
JOIN cols c ON c.table_schema = t.table_schema AND c.table_name = t.table_name
WHERE t.table_catalog = current_database()
  AND t.table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY t.table_schema, t.table_name, c.ordinal_position`

	rows, err := d.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("postgres: introspect columns: %w", err)
	}
	b := build.New()
	for rows.Next() {
		var ns, tbl, tblType, col, dtype, nullable string
		var def sql.NullString
		var ord int
		if err := rows.Scan(&ns, &tbl, &tblType, &col, &dtype, &nullable, &def, &ord); err != nil {
			rows.Close()
			return nil, fmt.Errorf("postgres: introspect scan: %w", err)
		}
		isView := tblType == "VIEW"
		c := schema.Column{Name: col, DataType: dtype, Nullable: nullable == "YES", Ordinal: ord}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		b.AddColumn(ns, tbl, isView, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("postgres: introspect rows: %w", err)
	}
	rows.Close()

	if err := d.loadPostgresConstraints(ctx, b); err != nil {
		return nil, err
	}
	if err := d.loadPostgresIndexes(ctx, b); err != nil {
		return nil, err
	}

	return b.Build(opts.Database), nil
}

func (d *postgresDriver) loadPostgresConstraints(ctx context.Context, b *build.Builder) error {
	const pkQ = `
SELECT tc.table_schema, tc.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND tc.table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY tc.table_schema, tc.table_name, kcu.ordinal_position`

	rows, err := d.db.QueryContext(ctx, pkQ)
	if err != nil {
		return fmt.Errorf("postgres: introspect pk: %w", err)
	}
	for rows.Next() {
		var ns, tbl, col string
		if err := rows.Scan(&ns, &tbl, &col); err != nil {
			rows.Close()
			return fmt.Errorf("postgres: introspect pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(ns, tbl, col)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("postgres: introspect pk rows: %w", err)
	}
	rows.Close()

	const fkQ = `
SELECT tc.table_schema, tc.table_name, tc.constraint_name,
       kcu.column_name, ccu.table_name AS ref_table, ccu.column_name AS ref_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND tc.table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY tc.table_schema, tc.table_name, tc.constraint_name, kcu.ordinal_position`

	frows, err := d.db.QueryContext(ctx, fkQ)
	if err != nil {
		return fmt.Errorf("postgres: introspect fk: %w", err)
	}
	defer frows.Close()
	for frows.Next() {
		var ns, tbl, name, col, refTbl, refCol string
		if err := frows.Scan(&ns, &tbl, &name, &col, &refTbl, &refCol); err != nil {
			return fmt.Errorf("postgres: introspect fk scan: %w", err)
		}
		b.AddForeignKeyColumn(ns, tbl, name, col, refTbl, refCol)
	}
	return frows.Err()
}

func (d *postgresDriver) loadPostgresIndexes(ctx context.Context, b *build.Builder) error {
	const q = `
SELECT schemaname, tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname NOT IN ('pg_catalog', 'information_schema')
ORDER BY schemaname, tablename, indexname`

	rows, err := d.db.QueryContext(ctx, q)
	if err != nil {
		return fmt.Errorf("postgres: introspect indexes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ns, tbl, name, def string
		if err := rows.Scan(&ns, &tbl, &name, &def); err != nil {
			return fmt.Errorf("postgres: introspect index scan: %w", err)
		}
		// indexdef is authoritative for uniqueness; column extraction beyond the
		// relational core is deferred (Attributes carries the raw def).
		unique := strings.Contains(def, "CREATE UNIQUE INDEX")
		b.AddIndex(ns, tbl, schema.Index{Name: name, Unique: unique, Attributes: map[string]any{"definition": def}})
	}
	return rows.Err()
}

func scanRows(rows *sql.Rows) (*result.ResultSet, error) {
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("column types: %w", err)
	}

	rs := &result.ResultSet{}
	for _, ct := range colTypes {
		nullable, _ := ct.Nullable()
		rs.Columns = append(rs.Columns, result.Column{
			Name:     ct.Name(),
			Type:     result.NormalizeColumnType(ct.DatabaseTypeName()),
			RawType:  ct.DatabaseTypeName(),
			Nullable: nullable,
		})
	}

	for rows.Next() {
		receivers := make([]any, len(colTypes))
		ptrs := make([]any, len(colTypes))
		for i := range receivers {
			ptrs[i] = &receivers[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(result.Row, len(colTypes))
		for i, val := range receivers {
			row[i] = toValue(val)
		}
		rs.Rows = append(rs.Rows, row)
	}
	return rs, rows.Err()
}

func toValue(v any) result.Value {
	if v == nil {
		return result.Value{Type: result.ValueTypeNull}
	}
	switch val := v.(type) {
	case int64:
		return result.Value{Type: result.ValueTypeInteger, Integer: val}
	case float64:
		return result.Value{Type: result.ValueTypeFloat, Float: val}
	case bool:
		return result.Value{Type: result.ValueTypeBool, Bool: val}
	case time.Time:
		utc := val.UTC()
		return result.Value{Type: result.ValueTypeTime, Time: &utc}
	case []byte:
		if utf8.Valid(val) {
			return result.Value{Type: result.ValueTypeText, Text: string(val)}
		}
		return result.Value{Type: result.ValueTypeBytes, Bytes: val}
	case string:
		return result.Value{Type: result.ValueTypeText, Text: val}
	default:
		return result.Value{Type: result.ValueTypeText, Text: fmt.Sprintf("%v", val)}
	}
}
