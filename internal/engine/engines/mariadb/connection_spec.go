package mariadb

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/mysql"
)

var _ engine.ConnectionSpec = (*driver)(nil)

func (*driver) Fields() []engine.FieldSpec { return mysql.ConnectionFields("3306") }

func (*driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	return mysql.BuildConnectionDSN(params, secrets)
}

func (*driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	return mysql.ParseConnectionDSN(dsn, "3306")
}

func (*driver) LegacyTLSMode(native string) engine.TLSMode { return mysql.LegacyTLSMode(native) }
