package oracle

import "github.com/sqlwarden/internal/engine"

// oracleDriver must implement the engine connection contract.
var _ engine.Driver = (*oracleDriver)(nil)

func init() {
	engine.Register(engine.Registration{
		ID:          "oracle",
		DisplayName: "Oracle",
		Dialect:     engine.DialectOracle,
		New:         func() engine.Driver { return &oracleDriver{} },
	})
}
