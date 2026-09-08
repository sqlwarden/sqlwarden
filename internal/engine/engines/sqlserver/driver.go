package sqlserver

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"

	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
)

// Driver is the SQL Server engine.Driver implementation.
type Driver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

// DB returns the underlying connection pool. Catalog/DDL introspection always
// runs directly against it (never through an open transaction).
func (d *Driver) DB() *sql.DB {
	return d.db
}

// DefaultScope returns the connection's configured default scope (database).
func (d *Driver) DefaultScope() metadata.ScopePath {
	return d.defaultScope
}

type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *Driver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

func (d *Driver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	params, err := msdsn.Parse(cfg.DSN)
	if err != nil {
		return fmt.Errorf("sqlserver: parse dsn: %w", err)
	}
	if selectedDatabase := cfg.DefaultScope.Name("database"); selectedDatabase != "" {
		params.Database = selectedDatabase
	}
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return fmt.Errorf("sqlserver: tls config: %w", err)
	}
	if tlsCfg != nil {
		if tlsCfg.ServerName == "" {
			tlsCfg.ServerName = params.Host
		}
		params.Encryption = msdsn.EncryptionRequired
		params.TLSConfig = tlsCfg
	}

	connector := mssql.NewConnectorConfig(params)
	if cfg.SSHDialer != nil {
		connector.Dialer = sshDialer(cfg.SSHDialer)
	}

	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("sqlserver: ping: %w", err)
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	d.defaultScope = cfg.DefaultScope
	return nil
}

// sshDialer adapts SQLWarden's dial-func SSH tunnel signature to
// go-mssqldb's mssql.Dialer interface.
type sshDialer func(ctx context.Context, network, addr string) (net.Conn, error)

func (f sshDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}

func (d *Driver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *Driver) Close() error {
	return d.db.Close()
}

func (d *Driver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: query: %w", err)
	}
	return cursor.ScanRows(rows, d.scanOptions)
}

func (d *Driver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *Driver) Dialect() engine.Dialect {
	return engine.DialectSQLServer
}
