package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	driver.Register("mysql", func() driver.Driver { return &mysqlDriver{} })
}

type mysqlDriver struct {
	db *sql.DB
}

// ensureParams ensures parseTime=true and tinyint1isBool=true are in the DSN.
func ensureParams(dsn string) string {
	params := map[string]string{
		"parseTime":     "true",
		"tinyint1isBool": "true",
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

func (d *mysqlDriver) Tables(ctx context.Context, database, schema string) ([]driver.TableMeta, error) {
	query := `
		SELECT table_name, table_schema FROM information_schema.tables
		WHERE table_schema = DATABASE()
		ORDER BY table_schema, table_name`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("mysql: tables: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableMeta
	for rows.Next() {
		var t driver.TableMeta
		if err := rows.Scan(&t.Name, &t.Schema); err != nil {
			return nil, fmt.Errorf("mysql: tables scan: %w", err)
		}
		if schema != "" && t.Schema != schema {
			continue
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (d *mysqlDriver) Columns(ctx context.Context, database, schema, table string) ([]driver.ColumnMeta, error) {
	query := `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		ORDER BY ordinal_position`

	rows, err := d.db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}
	defer rows.Close()

	var cols []driver.ColumnMeta
	for rows.Next() {
		var c driver.ColumnMeta
		var isNullable string
		if err := rows.Scan(&c.Name, &c.Type, &isNullable); err != nil {
			return nil, fmt.Errorf("mysql: columns scan: %w", err)
		}
		c.Nullable = isNullable == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func (d *mysqlDriver) Dialect() driver.Dialect {
	return driver.DialectMySQL
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
