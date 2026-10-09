package sqlite

import (
	"errors"
	"net/url"
	"strings"

	"github.com/sqlwarden/internal/engine"
)

var _ engine.ConnectionSpec = (*sqliteDriver)(nil)

func (*sqliteDriver) Fields() []engine.FieldSpec {
	return []engine.FieldSpec{
		{Key: "path", Label: "Database file path", Type: engine.FieldTypeString, Required: true},
	}
}

func (*sqliteDriver) BuildDSN(params engine.Params, _ engine.Secrets) (string, error) {
	path := params["path"]
	if path == "" {
		return "", errors.New("sqlite connection field path is required")
	}
	if strings.ContainsAny(path, "?#") {
		return "", errors.New("sqlite path contains an unsupported delimiter")
	}
	escapedPath := strings.ReplaceAll(url.PathEscape(path), "%2F", "/")
	return "file:" + escapedPath, nil
}

func (*sqliteDriver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "file" || u.Host != "" {
		return nil, nil, errors.New("invalid sqlite connection string")
	}
	if err := engine.RejectUnsupportedDSNParameters(u.Query()); err != nil {
		return nil, nil, err
	}
	path := u.Path
	if path == "" {
		path, err = url.PathUnescape(u.Opaque)
		if err != nil {
			return nil, nil, errors.New("invalid sqlite connection string")
		}
	}
	if path == "" {
		return nil, nil, errors.New("invalid sqlite connection string")
	}
	return engine.Params{"path": path}, engine.Secrets{}, nil
}
