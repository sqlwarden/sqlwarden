package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/go-sql-driver/mysql"
)

type mysqlDriver struct {
	db          *sql.DB
	scanOptions cursor.ScanOptions
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

func (d *mysqlDriver) Connect(ctx context.Context, cfg dbengine.ConnectionConfig) error {
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
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
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
	return cursor.ScanRows(rows, d.scanOptions)
}

func (d *mysqlDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: execute: %w", err)
	}
	return cursor.ScanRows(rows, d.scanOptions)
}

func (d *mysqlDriver) Dialect() dbengine.Dialect {
	return dbengine.DialectMySQL
}
