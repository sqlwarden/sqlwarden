package sqlserver

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

var _ engine.ConnectionSpec = (*Driver)(nil)

func (*Driver) Fields() []engine.FieldSpec {
	return []engine.FieldSpec{
		{Key: "host", Label: "Host", Type: engine.FieldTypeString, Required: true},
		{Key: "port", Label: "Port", Type: engine.FieldTypeInt, Required: true, Default: "1433"},
		{Key: "database", Label: "Database (optional)", Type: engine.FieldTypeString, NonNetwork: true},
		{Key: "username", Label: "Username", Type: engine.FieldTypeString, Required: true, NonNetwork: true},
		{Key: "password", Label: "Password", Type: engine.FieldTypeString, Secret: true},
	}
}

func (*Driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	for _, key := range []string{"host", "port", "username"} {
		if params[key] == "" {
			return "", errors.New("sqlserver connection field " + key + " is required")
		}
	}
	u := &url.URL{Scheme: "sqlserver", Host: net.JoinHostPort(params["host"], params["port"])}
	if password := secrets["password"]; password != "" {
		u.User = url.UserPassword(params["username"], password)
	} else {
		u.User = url.User(params["username"])
	}
	if database := params["database"]; database != "" {
		query := url.Values{"database": []string{database}}
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func (*Driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "sqlserver" || u.Hostname() == "" || u.User == nil {
		return nil, nil, errors.New("invalid sqlserver connection string")
	}
	if err := engine.RejectUnsupportedDSNParameters(u.Query(), "database", "encrypt"); err != nil {
		return nil, nil, err
	}
	port := u.Port()
	if port == "" {
		port = "1433"
	}
	params := engine.Params{
		"host": u.Hostname(), "port": port, "database": u.Query().Get("database"), "username": u.User.Username(),
	}
	if tlsMode := u.Query().Get("encrypt"); tlsMode != "" {
		params["tls_mode"] = tlsMode
	}
	secrets := engine.Secrets{}
	if password, present := u.User.Password(); present && password != "" {
		secrets["password"] = password
	}
	return params, secrets, nil
}

// LegacyTLSMode maps a SQL Server encrypt value to a structured TLS mode.
func (*Driver) LegacyTLSMode(native string) engine.TLSMode {
	switch strings.ToLower(strings.TrimSpace(native)) {
	case "", "false", "disable", "no", "f", "0":
		return engine.TLSModeDisable
	case "true", "strict", "yes", "t", "1":
		return engine.TLSModeVerifyFull
	default:
		return engine.TLSModeRequire
	}
}
