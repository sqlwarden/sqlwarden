package enginetest_test

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/pkg/result"
)

type selectOneDriver struct{}

func (selectOneDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (selectOneDriver) Ping(context.Context) error                             { return nil }
func (selectOneDriver) Close() error                                           { return nil }
func (selectOneDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{Rows: []result.Row{{}}}, nil
}
func (selectOneDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (selectOneDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

func TestHarnessAcceptsValidEngine(t *testing.T) {
	engine.Register(engine.Registration{
		ID: "harness-fake", DisplayName: "Harness Fake", Dialect: engine.DialectSQLite,
		New: func() engine.Driver { return selectOneDriver{} },
	})
	enginetest.RunCapabilityContract(t, "harness-fake")
	enginetest.RunConnectionContract(t, "harness-fake", engine.ConnectionConfig{DSN: "ignored", Driver: "harness-fake"})
}
