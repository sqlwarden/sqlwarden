package mysql

import "github.com/sqlwarden/internal/engine"

// mysqlDriver must implement the engine connection contract.
var (
	_ engine.Driver     = (*mysqlDriver)(nil)
	_ engine.TLSCapable = (*mysqlDriver)(nil)
)

// TLSSpec advertises the TLS material MySQL accepts. The go-sql-driver receives
// a *tls.Config through its named-registry, so every mode and field applies.
func (d *mysqlDriver) TLSSpec() engine.TLSSpec {
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
		New:         func() engine.Driver { return &mysqlDriver{} },
	})
}
