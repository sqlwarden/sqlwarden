package oracle

import "github.com/sqlwarden/internal/engine"

// oracleDriver must implement the engine connection contract.
var (
	_ engine.Driver           = (*oracleDriver)(nil)
	_ engine.TLSCapable       = (*oracleDriver)(nil)
	_ engine.SSHTunnelCapable = (*oracleDriver)(nil)
)

// SupportsSSHTunnel reports that go-ora accepts a DialerContext on its
// connector, so the transport can be routed through an SSH bastion.
func (d *oracleDriver) SupportsSSHTunnel() bool { return true }

// TLSSpec advertises the TLS material Oracle accepts. go-ora clobbers the
// tls.Config ServerName with the host address during negotiation, so a
// server-name override cannot take effect and is not advertised.
func (d *oracleDriver) TLSSpec() engine.TLSSpec {
	return engine.TLSSpec{
		Modes: []engine.TLSMode{
			engine.TLSModeDisable, engine.TLSModeRequire,
			engine.TLSModeVerifyCA, engine.TLSModeVerifyFull,
		},
		SupportsCABundle:   true,
		SupportsClientCert: true,
		SupportsServerName: false,
	}
}

func init() {
	engine.Register(engine.Registration{
		ID:          "oracle",
		DisplayName: "Oracle",
		Dialect:     engine.DialectOracle,
		New:         func() engine.Driver { return &oracleDriver{} },
	})
}
