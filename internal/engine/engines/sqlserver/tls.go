package sqlserver

import "github.com/sqlwarden/internal/engine"

var (
	_ engine.TLSCapable       = (*Driver)(nil)
	_ engine.SSHTunnelCapable = (*Driver)(nil)
)

// SupportsSSHTunnel reports that Connect routes the connector's Dialer
// through an SSH bastion when cfg.SSHDialer is set.
func (d *Driver) SupportsSSHTunnel() bool { return true }

// TLSSpec advertises the TLS material SQL Server accepts. go-mssqldb takes a
// *tls.Config directly on msdsn.Config (no process-global registry, unlike
// go-sql-driver/mysql), so every mode and field applies.
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
