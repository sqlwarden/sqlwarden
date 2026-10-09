package sqlite

import (
	"strings"

	"github.com/sqlwarden/internal/engine"
)

// sqliteDriver must implement the engine connection contract.
var _ engine.Driver = (*sqliteDriver)(nil)
var _ engine.TargetClassifier = (*sqliteDriver)(nil)

func (*sqliteDriver) TargetKind(dsn string) engine.TargetKind {
	dsn = strings.TrimSpace(dsn)
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:") {
		return engine.TargetKindInMemory
	}
	return engine.TargetKindLocal
}

func init() {
	engine.Register(engine.Registration{
		ID:          "sqlite",
		DisplayName: "SQLite",
		Dialect:     engine.DialectSQLite,
		New:         func() engine.Driver { return &sqliteDriver{} },
	})
}
