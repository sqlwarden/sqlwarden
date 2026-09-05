package postgres

import "github.com/sqlwarden/internal/engine"

// Driver must implement the engine connection contract.
var (
	_ engine.Driver           = (*Driver)(nil)
	_ engine.TLSCapable       = (*Driver)(nil)
	_ engine.SSHTunnelCapable = (*Driver)(nil)
)

// SupportsSSHTunnel reports that pgx accepts a custom context dialer
// (config.DialFunc), so the transport can be routed through an SSH bastion.
func (d *Driver) SupportsSSHTunnel() bool { return true }

// TLSSpec advertises the TLS material PostgreSQL accepts. pgx applies the
// resulting *tls.Config directly, so every mode and every field is supported.
func (d *Driver) TLSSpec() engine.TLSSpec {
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
		New:         func() engine.Driver { return &Driver{} },
	})
}
