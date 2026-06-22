package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/sqlwarden/internal/driver"
	build "github.com/sqlwarden/internal/driver/internal/build"
	"github.com/sqlwarden/internal/schema"
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

func (d *sqliteDriver) Dialect() driver.Dialect {
	return driver.DialectSQLite
}

func (d *sqliteDriver) Introspect(ctx context.Context, opts schema.IntrospectOptions) (*schema.Schema, error) {
	const ns = "main"
	b := build.New()

	type obj struct {
		name   string
		isView bool
	}
	var objects []obj

	rows, err := d.db.QueryContext(ctx,
		`SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: introspect objects: %w", err)
	}
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sqlite: introspect objects scan: %w", err)
		}
		objects = append(objects, obj{name: name, isView: typ == "view"})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("sqlite: introspect objects rows: %w", err)
	}
	rows.Close()

	for _, o := range objects {
		// Columns + primary key. PRAGMA table_info: cid, name, type, notnull, dflt_value, pk.
		cinfo, err := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", o.name))
		if err != nil {
			return nil, fmt.Errorf("sqlite: introspect columns %s: %w", o.name, err)
		}
		type pkCol struct {
			name string
			pos  int
		}
		var pks []pkCol
		for cinfo.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt sql.NullString
			if err := cinfo.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				cinfo.Close()
				return nil, fmt.Errorf("sqlite: introspect column scan %s: %w", o.name, err)
			}
			c := schema.Column{Name: name, DataType: ctype, Nullable: notnull == 0, Ordinal: cid + 1}
			if dflt.Valid {
				v := dflt.String
				c.Default = &v
			}
			b.AddColumn(ns, o.name, o.isView, c)
			if pk > 0 {
				pks = append(pks, pkCol{name: name, pos: pk})
			}
		}
		if err := cinfo.Err(); err != nil {
			cinfo.Close()
			return nil, fmt.Errorf("sqlite: introspect column rows %s: %w", o.name, err)
		}
		cinfo.Close()

		sort.Slice(pks, func(i, j int) bool { return pks[i].pos < pks[j].pos })
		for _, p := range pks {
			b.AddPrimaryKeyColumn(ns, o.name, p.name)
		}

		if o.isView {
			continue
		}

		// Foreign keys. PRAGMA foreign_key_list: id, seq, table, from, to, on_update, on_delete, match.
		fkInfo, err := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", o.name))
		if err != nil {
			return nil, fmt.Errorf("sqlite: introspect fk %s: %w", o.name, err)
		}
		for fkInfo.Next() {
			var id, seq int
			var refTbl, from, to, onUpdate, onDelete, match string
			if err := fkInfo.Scan(&id, &seq, &refTbl, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				fkInfo.Close()
				return nil, fmt.Errorf("sqlite: introspect fk scan %s: %w", o.name, err)
			}
			fkName := fmt.Sprintf("%s_fk_%d", o.name, id)
			b.AddForeignKeyColumn(ns, o.name, fkName, from, refTbl, to)
		}
		if err := fkInfo.Err(); err != nil {
			fkInfo.Close()
			return nil, fmt.Errorf("sqlite: introspect fk rows %s: %w", o.name, err)
		}
		fkInfo.Close()

		// Indexes. PRAGMA index_list: seq, name, unique, origin, partial.
		idxInfo, err := d.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%q)", o.name))
		if err != nil {
			return nil, fmt.Errorf("sqlite: introspect index %s: %w", o.name, err)
		}
		for idxInfo.Next() {
			var seq, unique, partial int
			var name, origin string
			if err := idxInfo.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
				idxInfo.Close()
				return nil, fmt.Errorf("sqlite: introspect index scan %s: %w", o.name, err)
			}
			b.AddIndex(ns, o.name, schema.Index{Name: name, Unique: unique == 1})
		}
		if err := idxInfo.Err(); err != nil {
			idxInfo.Close()
			return nil, fmt.Errorf("sqlite: introspect index rows %s: %w", o.name, err)
		}
		idxInfo.Close()
	}

	// Triggers.
	b.DeclareGroup("trigger", "Triggers")
	trows, err := d.db.QueryContext(ctx,
		`SELECT name, tbl_name FROM sqlite_master WHERE type = 'trigger' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: introspect triggers: %w", err)
	}
	for trows.Next() {
		var name, tbl string
		if err := trows.Scan(&name, &tbl); err != nil {
			trows.Close()
			return nil, fmt.Errorf("sqlite: introspect triggers scan: %w", err)
		}
		b.AddObject(ns, "trigger", name)
		b.SetObjectAttribute(ns, "trigger", name, "table", tbl)
	}
	if err := trows.Err(); err != nil {
		trows.Close()
		return nil, fmt.Errorf("sqlite: introspect triggers rows: %w", err)
	}
	trows.Close()

	return b.Build(""), nil
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
