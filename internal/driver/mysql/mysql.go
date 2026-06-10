package mysql

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

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	driver.Register("mysql", func() driver.Driver { return &mysqlDriver{} })
}

type mysqlDriver struct {
	db *sql.DB
}

// ensureParams ensures parseTime=true is in the DSN.
func ensureParams(dsn string) string {
	params := map[string]string{
		"parseTime": "true",
	}

	// Split DSN into base and query string parts.
	// MySQL DSN format: [user[:password]@][net[(addr)]]/dbname[?param1=value1&...]
	sep := strings.LastIndex(dsn, "?")
	var base, query string
	if sep == -1 {
		base = dsn
		query = ""
	} else {
		base = dsn[:sep]
		query = dsn[sep+1:]
	}

	existing := map[string]bool{}
	if query != "" {
		for part := range strings.SplitSeq(query, "&") {
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
				existing[kv[0]] = true
			}
		}
	}

	var extra []string
	for k, v := range params {
		if !existing[k] {
			extra = append(extra, k+"="+v)
		}
	}

	if len(extra) == 0 {
		return dsn
	}

	addition := strings.Join(extra, "&")
	if query == "" {
		return base + "?" + addition
	}
	return base + "?" + query + "&" + addition
}

func (d *mysqlDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error {
	dsn := ensureParams(cfg.DSN)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql: ping: %w", err)
	}
	d.db = db
	return nil
}

func (d *mysqlDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *mysqlDriver) Close() error {
	return d.db.Close()
}

func (d *mysqlDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *mysqlDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: execute: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *mysqlDriver) Dialect() driver.Dialect {
	return driver.DialectMySQL
}

func (d *mysqlDriver) Introspect(ctx context.Context, opts schema.IntrospectOptions) (*schema.Schema, error) {
	b := build.New()
	ns := currentDatabase(ctx, d.db)

	const colQ = `
SELECT t.table_name, t.table_type, c.column_name, c.column_type,
       c.is_nullable, c.column_default, c.ordinal_position
FROM information_schema.tables t
JOIN information_schema.columns c
  ON c.table_schema = t.table_schema AND c.table_name = t.table_name
WHERE t.table_schema = DATABASE()
ORDER BY t.table_name, c.ordinal_position`

	rows, err := d.db.QueryContext(ctx, colQ)
	if err != nil {
		return nil, fmt.Errorf("mysql: introspect columns: %w", err)
	}
	for rows.Next() {
		var tbl, tblType, col, ctype, nullable string
		var def sql.NullString
		var ord int
		if err := rows.Scan(&tbl, &tblType, &col, &ctype, &nullable, &def, &ord); err != nil {
			rows.Close()
			return nil, fmt.Errorf("mysql: introspect scan: %w", err)
		}
		c := schema.Column{Name: col, DataType: ctype, Nullable: nullable == "YES", Ordinal: ord}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		b.AddColumn(ns, tbl, tblType == "VIEW", c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("mysql: introspect rows: %w", err)
	}
	rows.Close()

	const pkQ = `
SELECT kcu.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name
 AND kcu.table_schema = tc.table_schema
 AND kcu.table_name = tc.table_name
WHERE tc.table_schema = DATABASE() AND tc.constraint_type = 'PRIMARY KEY'
ORDER BY kcu.table_name, kcu.ordinal_position`

	pkRows, err := d.db.QueryContext(ctx, pkQ)
	if err != nil {
		return nil, fmt.Errorf("mysql: introspect pk: %w", err)
	}
	for pkRows.Next() {
		var tbl, col string
		if err := pkRows.Scan(&tbl, &col); err != nil {
			pkRows.Close()
			return nil, fmt.Errorf("mysql: introspect pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(ns, tbl, col)
	}
	if err := pkRows.Err(); err != nil {
		pkRows.Close()
		return nil, fmt.Errorf("mysql: introspect pk rows: %w", err)
	}
	pkRows.Close()

	const fkQ = `
SELECT kcu.table_name, kcu.constraint_name, kcu.column_name,
       kcu.referenced_table_name, kcu.referenced_column_name
FROM information_schema.key_column_usage kcu
WHERE kcu.table_schema = DATABASE() AND kcu.referenced_table_name IS NOT NULL
ORDER BY kcu.table_name, kcu.constraint_name, kcu.ordinal_position`

	fkRows, err := d.db.QueryContext(ctx, fkQ)
	if err != nil {
		return nil, fmt.Errorf("mysql: introspect fk: %w", err)
	}
	for fkRows.Next() {
		var tbl, name, col, refTbl, refCol string
		if err := fkRows.Scan(&tbl, &name, &col, &refTbl, &refCol); err != nil {
			fkRows.Close()
			return nil, fmt.Errorf("mysql: introspect fk scan: %w", err)
		}
		b.AddForeignKeyColumn(ns, tbl, name, col, refTbl, refCol)
	}
	if err := fkRows.Err(); err != nil {
		fkRows.Close()
		return nil, fmt.Errorf("mysql: introspect fk rows: %w", err)
	}
	fkRows.Close()

	const idxQ = `
SELECT table_name, index_name, non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
GROUP BY table_name, index_name, non_unique
ORDER BY table_name, index_name`

	idxRows, err := d.db.QueryContext(ctx, idxQ)
	if err != nil {
		return nil, fmt.Errorf("mysql: introspect indexes: %w", err)
	}
	defer idxRows.Close()
	for idxRows.Next() {
		var tbl, name string
		var nonUnique int
		if err := idxRows.Scan(&tbl, &name, &nonUnique); err != nil {
			return nil, fmt.Errorf("mysql: introspect index scan: %w", err)
		}
		b.AddIndex(ns, tbl, schema.Index{Name: name, Unique: nonUnique == 0})
	}
	if err := idxRows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: introspect index rows: %w", err)
	}

	return b.Build(ns), nil
}

func currentDatabase(ctx context.Context, db *sql.DB) string {
	var name sql.NullString
	_ = db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name)
	return name.String
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
