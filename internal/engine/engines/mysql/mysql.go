package mysql

import "github.com/sqlwarden/internal/engine"

// Driver must implement the engine connection contract.
var (
	_ engine.Driver           = (*Driver)(nil)
	_ engine.TLSCapable       = (*Driver)(nil)
	_ engine.SSHTunnelCapable = (*Driver)(nil)
)

// SupportsSSHTunnel reports that go-sql-driver/mysql accepts a registered
// DialContext function, so the transport can be routed through an SSH bastion.
func (d *Driver) SupportsSSHTunnel() bool { return true }

// TLSSpec advertises the TLS material MySQL accepts. The go-sql-driver receives
// a *tls.Config through its named-registry, so every mode and field applies.
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
		ID:          "mysql",
		DisplayName: "MySQL",
		Dialect:     engine.DialectMySQL,
		New:         func() engine.Driver { return &Driver{} },
	})
}
