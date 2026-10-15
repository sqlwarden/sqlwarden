package mysql

import (
	"errors"
	"net"
	"net/url"
	"strings"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/sqlwarden/internal/engine"
)

var _ engine.ConnectionSpec = (*Driver)(nil)

// ConnectionFields returns MySQL-wire connection fields with the supplied
// port default for compatible engines.
func ConnectionFields(defaultPort string) []engine.FieldSpec {
	return []engine.FieldSpec{
		{Key: "host", Label: "Host", Type: engine.FieldTypeString, Required: true},
		{Key: "port", Label: "Port", Type: engine.FieldTypeInt, Required: true, Default: defaultPort},
		{Key: "database", Label: "Database (optional)", Type: engine.FieldTypeString, NonNetwork: true},
		{Key: "username", Label: "Username", Type: engine.FieldTypeString, Required: true, NonNetwork: true},
		{Key: "password", Label: "Password", Type: engine.FieldTypeString, Secret: true},
	}
}

// BuildConnectionDSN builds the native go-sql-driver/mysql form. TLS is not
// copied from Params because engine.ConnectionConfig.TLS applies it separately.
func BuildConnectionDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	for _, key := range []string{"host", "port", "username"} {
		if params[key] == "" {
			return "", errors.New("mysql connection field " + key + " is required")
		}
	}
	if strings.ContainsAny(params["database"], "?/") {
		return "", errors.New("mysql database name contains an unsupported delimiter")
	}
	config := mysqlconfig.NewConfig()
	config.User = params["username"]
	config.Passwd = secrets["password"]
	config.Net = "tcp"
	config.Addr = net.JoinHostPort(params["host"], params["port"])
	config.DBName = params["database"]
	return config.FormatDSN(), nil
}

// ParseConnectionDSN parses the native MySQL DSN. A legacy tls query value is
// returned as Params["tls_mode"].
func ParseConnectionDSN(dsn, defaultPort string) (engine.Params, engine.Secrets, error) {
	rawAddress, err := rawTCPAddress(dsn)
	if err != nil {
		return nil, nil, errors.New("invalid mysql connection string")
	}
	host, port, err := splitTCPAddress(rawAddress, defaultPort)
	if err != nil {
		return nil, nil, errors.New("invalid mysql connection string")
	}
	if err := engine.RejectUnsupportedDSNParameters(dsnQuery(dsn), "tls"); err != nil {
		return nil, nil, err
	}
	config, err := mysqlconfig.ParseDSN(dsn)
	if err != nil || config.Net != "tcp" || config.User == "" {
		return nil, nil, errors.New("invalid mysql connection string")
	}
	params := engine.Params{
		"host": host, "port": port, "database": config.DBName, "username": config.User,
	}
	if config.TLSConfig != "" {
		params["tls_mode"] = config.TLSConfig
	}
	secrets := engine.Secrets{}
	if config.Passwd != "" {
		secrets["password"] = config.Passwd
	}
	return params, secrets, nil
}

// dsnQuery returns the parameters after the database name. The password may
// contain "?", so the query is located after the tcp address terminator.
func dsnQuery(dsn string) url.Values {
	const marker = ")/"
	start := strings.LastIndex(dsn, "@tcp(")
	if start < 0 {
		return nil
	}
	_, rest, found := strings.Cut(dsn[start:], marker)
	if !found {
		return nil
	}
	_, raw, found := strings.Cut(rest, "?")
	if !found {
		return nil
	}
	query := url.Values{}
	for _, pair := range strings.Split(raw, "&") {
		if key, _, _ := strings.Cut(pair, "="); key != "" {
			query[key] = nil
		}
	}
	return query
}

func rawTCPAddress(dsn string) (string, error) {
	const marker = "@tcp("
	start := strings.LastIndex(dsn, marker)
	if start < 0 {
		return "", errors.New("missing tcp address")
	}
	addressAndDatabase := dsn[start+len(marker):]
	end := strings.Index(addressAndDatabase, ")/")
	if end < 0 {
		return "", errors.New("invalid tcp address")
	}
	return addressAndDatabase[:end], nil
}

func splitTCPAddress(address, defaultPort string) (string, string, error) {
	if address == "" {
		return "", "", errors.New("empty tcp address")
	}
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		host := strings.TrimSuffix(strings.TrimPrefix(address, "["), "]")
		if host == "" {
			return "", "", errors.New("empty tcp host")
		}
		return host, defaultPort, nil
	}
	if net.ParseIP(address) != nil {
		return address, defaultPort, nil
	}
	if !strings.Contains(address, ":") {
		return address, defaultPort, nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return "", "", errors.New("invalid tcp address")
	}
	return host, port, nil
}

func (*Driver) Fields() []engine.FieldSpec { return ConnectionFields("3306") }

func (*Driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	return BuildConnectionDSN(params, secrets)
}

func (*Driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	return ParseConnectionDSN(dsn, "3306")
}

// LegacyTLSMode maps a MySQL tls DSN value to a structured TLS mode. Only the
// bare "true" value verifies the server certificate and host name.
func LegacyTLSMode(native string) engine.TLSMode {
	switch strings.ToLower(strings.TrimSpace(native)) {
	case "", "false":
		return engine.TLSModeDisable
	case "true":
		return engine.TLSModeVerifyFull
	default:
		return engine.TLSModeRequire
	}
}

func (*Driver) LegacyTLSMode(native string) engine.TLSMode { return LegacyTLSMode(native) }
