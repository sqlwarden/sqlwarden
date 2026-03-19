package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"

	_ "modernc.org/sqlite"
)

func init() {
	driver.Register("sqlite", func() driver.Driver { return &sqliteDriver{} })
}

type sqliteDriver struct {
	db *sql.DB
}

func (d *sqliteDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error {
	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return fmt.Errorf("sqlite: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("sqlite: ping: %w", err)
	}
	// Enable WAL mode for better concurrency.
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("sqlite: WAL mode: %w", err)
	}
	d.db = db
	return nil
}

func (d *sqliteDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *sqliteDriver) Close() error {
	return d.db.Close()
}

func (d *sqliteDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *sqliteDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: execute: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (d *sqliteDriver) Tables(ctx context.Context, database, schema string) ([]driver.TableMeta, error) {
	query := `SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: tables: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableMeta
	for rows.Next() {
		var t driver.TableMeta
		if err := rows.Scan(&t.Name); err != nil {
			return nil, fmt.Errorf("sqlite: tables scan: %w", err)
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (d *sqliteDriver) Columns(ctx context.Context, database, schema, table string) ([]driver.ColumnMeta, error) {
	// PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
	query := fmt.Sprintf("PRAGMA table_info(%q)", table)

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite: columns: %w", err)
	}
	defer rows.Close()

	var cols []driver.ColumnMeta
	for rows.Next() {
		var cid int
		var name, colType string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("sqlite: columns scan: %w", err)
		}
		cols = append(cols, driver.ColumnMeta{
			Name:     name,
			Type:     colType,
			Nullable: notnull == 0,
		})
	}
	return cols, rows.Err()
}

func (d *sqliteDriver) Dialect() driver.Dialect {
	return driver.DialectSQLite
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
