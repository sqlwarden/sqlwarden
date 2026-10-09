package postgres

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

var _ engine.ConnectionSpec = (*Driver)(nil)

// ConnectionFields returns PostgreSQL-wire connection fields with the supplied
// engine defaults. Compatible engines use it while retaining their own driver
// method sets and registration identities.
func ConnectionFields(defaultPort string, databaseRequired bool, databaseDefault, usernameDefault string) []engine.FieldSpec {
	databaseLabel := "Database"
	if !databaseRequired {
		databaseLabel = "Database (optional)"
	}
	return []engine.FieldSpec{
		{Key: "host", Label: "Host", Type: engine.FieldTypeString, Required: true},
		{Key: "port", Label: "Port", Type: engine.FieldTypeInt, Required: true, Default: defaultPort},
		{Key: "database", Label: databaseLabel, Type: engine.FieldTypeString, Required: databaseRequired, Default: databaseDefault, NonNetwork: true},
		{Key: "username", Label: "Username", Type: engine.FieldTypeString, Required: true, Default: usernameDefault, NonNetwork: true},
		{Key: "password", Label: "Password", Type: engine.FieldTypeString, Secret: true},
	}
}

// BuildConnectionDSN builds the URL form accepted by pgx without emitting TLS
// parameters, which are applied later from engine.ConnectionConfig.TLS.
func BuildConnectionDSN(params engine.Params, secrets engine.Secrets, databaseRequired bool) (string, error) {
	required := []string{"host", "port", "username"}
	if databaseRequired {
		required = append(required, "database")
	}
	for _, key := range required {
		if params[key] == "" {
			return "", errors.New("postgres connection field " + key + " is required")
		}
	}

	u := &url.URL{
		Scheme: "postgresql",
		Host:   net.JoinHostPort(params["host"], params["port"]),
		Path:   "/" + params["database"],
	}
	if password := secrets["password"]; password != "" {
		u.User = url.UserPassword(params["username"], password)
	} else {
		u.User = url.User(params["username"])
	}
	return u.String(), nil
}

// ParseConnectionDSN parses a PostgreSQL URL and applies defaultPort when the
// URL omits a port. A legacy sslmode is returned as Params["tls_mode"].
func ParseConnectionDSN(dsn, defaultPort string) (engine.Params, engine.Secrets, error) {
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil {
		return nil, nil, errors.New("invalid postgres connection string")
	}
	if err := engine.RejectUnsupportedDSNParameters(u.Query(), "sslmode"); err != nil {
		return nil, nil, err
	}
	port := u.Port()
	if port == "" {
		port = defaultPort
	}
	params := engine.Params{
		"host":     u.Hostname(),
		"port":     port,
		"database": strings.TrimPrefix(u.Path, "/"),
		"username": u.User.Username(),
	}
	if tlsMode := u.Query().Get("sslmode"); tlsMode != "" {
		params["tls_mode"] = tlsMode
	}
	secrets := engine.Secrets{}
	if password, present := u.User.Password(); present && password != "" {
		secrets["password"] = password
	}
	return params, secrets, nil
}

func (*Driver) Fields() []engine.FieldSpec {
	return ConnectionFields("5432", false, "", "")
}

func (*Driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	return BuildConnectionDSN(params, secrets, false)
}

func (*Driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	return ParseConnectionDSN(dsn, "5432")
}

// LegacyTLSMode maps a PostgreSQL sslmode value to a structured TLS mode.
func LegacyTLSMode(native string) engine.TLSMode {
	switch strings.ToLower(strings.TrimSpace(native)) {
	case "", "disable", "allow":
		return engine.TLSModeDisable
	case "verify-ca":
		return engine.TLSModeVerifyCA
	case "verify-full":
		return engine.TLSModeVerifyFull
	default:
		return engine.TLSModeRequire
	}
}

func (*Driver) LegacyTLSMode(native string) engine.TLSMode { return LegacyTLSMode(native) }
