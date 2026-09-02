package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/oklog/ulid/v2"
)

type mysqlDriver struct {
	db           *sql.DB
	currentTx    *sql.Tx
	scanOptions  cursor.ScanOptions
	defaultScope metadata.ScopePath
	// tlsName is the process-unique key this connection registered with the
	// mysql driver's global TLS registry; non-empty means Close must release it.
	tlsName string
	// netName is the process-unique custom-network key this connection
	// registered with the mysql driver for SSH-tunnel dialing; non-empty means
	// Close must deregister it.
	netName string
}

// applyTLS registers a process-unique *tls.Config with the mysql driver and
// rewrites the DSN to reference it by name. The name is recorded on the driver
// so releaseTLS (called from Close) can deregister it. A nil or "disable"
// config is a passthrough.
func (d *mysqlDriver) applyTLS(dsn string, tc *engine.TLSConfig) (string, error) {
	tlsCfg, err := tc.Build()
	if err != nil {
		return "", fmt.Errorf("mysql: tls config: %w", err)
	}
	if tlsCfg == nil {
		return dsn, nil
	}
	config, err := mysqlconfig.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("mysql: parse config: %w", err)
	}
	if tlsCfg.ServerName == "" {
		if host, _, ok := strings.Cut(config.Addr, ":"); ok {
			tlsCfg.ServerName = host
		} else {
			tlsCfg.ServerName = config.Addr
		}
	}
	name := "warden-tls-" + ulid.Make().String()
	if err := mysqlconfig.RegisterTLSConfig(name, tlsCfg); err != nil {
		return "", fmt.Errorf("mysql: register tls: %w", err)
	}
	d.tlsName = name
	config.TLSConfig = name
	return config.FormatDSN(), nil
}

// applySSHDialer registers a process-unique DialContext with the mysql driver
// and rewrites the DSN's network to reference it by name, so the connection's
// TCP transport is dialed through the SSH tunnel. A nil dialer is a passthrough.
func (d *mysqlDriver) applySSHDialer(dsn string, dialer func(ctx context.Context, network, addr string) (net.Conn, error)) (string, error) {
	if dialer == nil {
		return dsn, nil
	}
	config, err := mysqlconfig.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("mysql: parse config: %w", err)
	}
	name := "warden-ssh-" + ulid.Make().String()
	mysqlconfig.RegisterDialContext(name, func(ctx context.Context, addr string) (net.Conn, error) {
		return dialer(ctx, "tcp", addr)
	})
	d.netName = name
	config.Net = name
	return config.FormatDSN(), nil
}

// releaseRegistrations deregisters every process-global entry this connection
// created (TLS config, SSH custom network). Called from Close.
func (d *mysqlDriver) releaseRegistrations() {
	if d.tlsName != "" {
		mysqlconfig.DeregisterTLSConfig(d.tlsName)
		d.tlsName = ""
	}
	if d.netName != "" {
		mysqlconfig.DeregisterDialContext(d.netName)
		d.netName = ""
	}
}

type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (d *mysqlDriver) conn() execer {
	if d.currentTx != nil {
		return d.currentTx
	}
	return d.db
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

func (d *mysqlDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	dsn := ensureParams(cfg.DSN)
	if selectedDatabase := cfg.DefaultScope.Name("database"); selectedDatabase != "" {
		config, err := mysqlconfig.ParseDSN(dsn)
		if err != nil {
			return fmt.Errorf("mysql: parse config: %w", err)
		}
		config.DBName = selectedDatabase
		dsn = config.FormatDSN()
	}
	dsn, err := d.applyTLS(dsn, cfg.TLS)
	if err != nil {
		return err
	}
	dsn, err = d.applySSHDialer(dsn, cfg.SSHDialer)
	if err != nil {
		d.releaseRegistrations()
		return err
	}
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
	d.defaultScope = cfg.DefaultScope
	return nil
}

func (d *mysqlDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *mysqlDriver) Close() error {
	defer d.releaseRegistrations()
	return d.db.Close()
}

func (d *mysqlDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.QueryWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *mysqlDriver) QueryWithOptions(ctx context.Context, query string, opts cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	rows, err := d.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: query: %w", err)
	}
	return cursor.ScanRows(rows, opts)
}

func (d *mysqlDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	return d.ExecuteWithOptions(ctx, query, d.scanOptions, args...)
}

func (d *mysqlDriver) ExecuteWithOptions(ctx context.Context, query string, _ cursor.ScanOptions, args ...any) (*result.ResultSet, error) {
	// SQL is intentionally user-authored editor input and is permission-gated by the web layer.
	// codeql[go/sql-injection]
	execResult, err := d.conn().ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: execute: %w", err)
	}
	rowsAffected, err := execResult.RowsAffected()
	if err != nil {
		return &result.ResultSet{}, nil
	}
	return result.NewExecutionResult(rowsAffected), nil
}

func (d *mysqlDriver) Dialect() engine.Dialect {
	return engine.DialectMySQL
}
