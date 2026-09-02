package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strings"

	go_ora "github.com/sijms/go-ora/v2"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

type oracleDriver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
}

// execer is satisfied by both *sql.DB and *sql.Tx, letting every statement
// path resolve through the same accessor whether or not a transaction is open.
type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *oracleDriver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
}

// schemaConnector wraps a go-ora driver.Connector so that the configured
// default schema is an invariant of every session in the pool: sql.DB opens
// connections lazily and recycles them, so a one-off ALTER SESSION on a single
// connection would leave later connections resolving unqualified names against
// the login user's schema. quotedSchema is empty when no default schema is
// configured, in which case Connect is a passthrough.
type schemaConnector struct {
	inner        driver.Connector
	quotedSchema string
}

func (c schemaConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if c.quotedSchema == "" {
		return conn, nil
	}
	execer, ok := conn.(driver.ExecerContext)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("oracle: connection does not support session initialization")
	}
	// quotedSchema is oracle-quoted and sourced only from validated stored
	// connection config, so punctuation cannot alter the statement.
	// codeql[go/sql-injection]
	if _, err := execer.ExecContext(ctx, `ALTER SESSION SET CURRENT_SCHEMA = `+c.quotedSchema, nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("oracle: set current schema: %w", err)
	}
	return conn, nil
}

func (c schemaConnector) Driver() driver.Driver { return c.inner.Driver() }

// ensureOracleSSL forces the tcps protocol on an oracle:// URL by setting
// SSL=true. go-ora only negotiates TLS when the connect string selects it, so
// structured TLS material is inert without this even though WithTLSConfig is
// also set.
func ensureOracleSSL(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	if strings.EqualFold(q.Get("SSL"), "true") {
		return dsn
	}
	q.Set("SSL", "true")
	u.RawQuery = q.Encode()
	return u.String()
}

// buildOracleConnector assembles the go-ora connector, folding in the default
// schema and the structured TLS material. When TLS is configured the connect
// string is switched to tcps and the *tls.Config is handed to go-ora, which
// uses it verbatim for the handshake.
func buildOracleConnector(cfg engine.ConnectionConfig) (driver.Connector, error) {
	dsn := cfg.DSN
	tlsCfg, err := cfg.TLS.Build()
	if err != nil {
		return nil, fmt.Errorf("oracle: tls config: %w", err)
	}
	if tlsCfg != nil {
		dsn = ensureOracleSSL(dsn)
	}
	inner := go_ora.NewConnector(dsn)
	if tlsCfg != nil {
		oc, ok := inner.(*go_ora.OracleConnector)
		if !ok {
			return nil, fmt.Errorf("oracle: connector type %T does not accept TLS config", inner)
		}
		oc.WithTLSConfig(tlsCfg)
	}
	connector := schemaConnector{inner: inner}
	if schema := cfg.DefaultScope.Name("schema"); schema != "" {
		connector.quotedSchema = oracleQuoteIdent(schema)
	}
	return connector, nil
}

func (d *oracleDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	connector, err := buildOracleConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("oracle: ping: %w", err)
	}
	d.db = db
	d.scanOptions = cursor.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	d.defaultScope = cfg.DefaultScope
	return nil
}

func (d *oracleDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *oracleDriver) Close() error {
	return d.db.Close()
}

func (d *oracleDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *oracleDriver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *oracleDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *oracleDriver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *oracleDriver) Dialect() engine.Dialect {
	return engine.DialectOracle
}
