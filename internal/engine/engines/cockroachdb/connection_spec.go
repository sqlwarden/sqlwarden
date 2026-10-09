package cockroachdb

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

var _ engine.ConnectionSpec = (*driver)(nil)

func (*driver) Fields() []engine.FieldSpec {
	return postgres.ConnectionFields("26257", false, "", "")
}

func (*driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	return postgres.BuildConnectionDSN(params, secrets, false)
}

func (*driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	return postgres.ParseConnectionDSN(dsn, "26257")
}

func (*driver) LegacyTLSMode(native string) engine.TLSMode { return postgres.LegacyTLSMode(native) }
