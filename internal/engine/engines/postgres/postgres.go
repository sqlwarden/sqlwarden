package postgres

import "github.com/sqlwarden/internal/engine"

// postgresDriver must implement the engine connection contract.
var (
	_ engine.Driver           = (*postgresDriver)(nil)
	_ engine.TLSCapable       = (*postgresDriver)(nil)
	_ engine.SSHTunnelCapable = (*postgresDriver)(nil)
)

// SupportsSSHTunnel reports that pgx accepts a custom context dialer
// (config.DialFunc), so the transport can be routed through an SSH bastion.
func (d *postgresDriver) SupportsSSHTunnel() bool { return true }

// TLSSpec advertises the TLS material PostgreSQL accepts. pgx applies the
// resulting *tls.Config directly, so every mode and every field is supported.
func (d *postgresDriver) TLSSpec() engine.TLSSpec {
	return engine.TLSSpec{
		Modes: []engine.TLSMode{
			engine.TLSModeDisable, engine.TLSModeRequire,
			engine.TLSModeVerifyCA, engine.TLSModeVerifyFull,
		},
		SupportsCABundle:   true,
		SupportsClientCert: true,
		SupportsServerName: true,
	}
}

func init() {
	engine.Register(engine.Registration{
		ID:          "postgres",
		DisplayName: "PostgreSQL",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &postgresDriver{} },
	})
}
