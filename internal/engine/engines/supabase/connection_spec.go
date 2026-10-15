package supabase

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

var _ engine.ConnectionSpec = (*driver)(nil)

func (*driver) Fields() []engine.FieldSpec {
	return postgres.ConnectionFields("5432", true, "postgres", "postgres")
}

func (*driver) BuildDSN(params engine.Params, secrets engine.Secrets) (string, error) {
	return postgres.BuildConnectionDSN(params, secrets, true)
}

func (*driver) ParseDSN(dsn string) (engine.Params, engine.Secrets, error) {
	return postgres.ParseConnectionDSN(dsn, "5432")
}

func (*driver) LegacyTLSMode(native string) engine.TLSMode { return postgres.LegacyTLSMode(native) }
