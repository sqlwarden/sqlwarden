package oracle

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

var _ engine.ConnectionSpec = (*oracleDriver)(nil)

func (*oracleDriver) Fields() []engine.FieldSpec {
	return []engine.FieldSpec{
		{Key: "host", Label: "Host", Type: engine.FieldTypeString, Required: true},
		{Key: "port", Label: "Port", Type: engine.FieldTypeInt, Required: true, Default: "1521"},
		{Key: "serviceName", Label: "Service name", Type: engine.FieldTypeString, Required: true, NonNetwork: true},
		{Key: "username", Label: "Username", Type: engine.FieldTypeString, Required: true, NonNetwork: true},
		{Key: "password", Label: "Password", Type: engine.FieldTypeString, Secret: true},
	}
}

func (*oracleDriver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	for _, key := range []string{"host", "port", "serviceName", "username"} {
		if params[key] == "" {
			return "", errors.New("oracle connection field " + key + " is required")
		}
	}
	u := &url.URL{
		Scheme: "oracle",
		Host:   net.JoinHostPort(params["host"], params["port"]),
		Path:   "/" + params["serviceName"],
	}
	if password := secrets["password"]; password != "" {
		u.User = url.UserPassword(params["username"], password)
	} else {
		u.User = url.User(params["username"])
	}
	return u.String(), nil
}

func (*oracleDriver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "oracle" || u.Hostname() == "" || u.User == nil || strings.TrimPrefix(u.Path, "/") == "" {
		return nil, nil, errors.New("invalid oracle connection string")
	}
	if err := engine.RejectUnsupportedDSNParameters(u.Query(), "SSL"); err != nil {
		return nil, nil, err
	}
	port := u.Port()
	if port == "" {
		port = "1521"
	}
	params := engine.Params{
		"host": u.Hostname(), "port": port, "serviceName": strings.TrimPrefix(u.Path, "/"), "username": u.User.Username(),
	}
	if tlsMode := u.Query().Get("SSL"); tlsMode != "" {
		params["tls_mode"] = tlsMode
	}
	secrets := engine.Secrets{}
	if password, present := u.User.Password(); present && password != "" {
		secrets["password"] = password
	}
	return params, secrets, nil
}

// LegacyTLSMode maps an Oracle SSL value to a structured TLS mode.
func (*oracleDriver) LegacyTLSMode(native string) engine.TLSMode {
	switch strings.ToLower(strings.TrimSpace(native)) {
	case "", "false", "disable", "no":
		return engine.TLSModeDisable
	default:
		return engine.TLSModeRequire
	}
}
