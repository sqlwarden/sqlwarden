package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/driver"
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

func (d *postgresDriver) Tables(ctx context.Context, database, schema string) ([]driver.TableMeta, error) {
	query := `
		SELECT table_name, table_schema FROM information_schema.tables
		WHERE table_catalog = current_database()
		  AND table_schema NOT IN ('pg_catalog', 'information_schema')
		ORDER BY table_schema, table_name`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: tables: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableMeta
	for rows.Next() {
		var t driver.TableMeta
		if err := rows.Scan(&t.Name, &t.Schema); err != nil {
			return nil, fmt.Errorf("postgres: tables scan: %w", err)
		}
		if schema != "" && t.Schema != schema {
			continue
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (d *postgresDriver) Columns(ctx context.Context, database, schema, table string) ([]driver.ColumnMeta, error) {
	query := `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_catalog = current_database()
		  AND table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`

	rows, err := d.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, fmt.Errorf("postgres: columns: %w", err)
	}
	defer rows.Close()

	var cols []driver.ColumnMeta
	for rows.Next() {
		var c driver.ColumnMeta
		var isNullable string
		if err := rows.Scan(&c.Name, &c.Type, &isNullable); err != nil {
			return nil, fmt.Errorf("postgres: columns scan: %w", err)
		}
		c.Nullable = isNullable == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func (d *postgresDriver) Dialect() driver.Dialect {
	return driver.DialectPostgres
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
