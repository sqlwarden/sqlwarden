package enginetest_test

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/enginetest"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"
)

type selectOneDriver struct{}

func (selectOneDriver) Connect(context.Context, driver.ConnectionConfig) error { return nil }
func (selectOneDriver) Ping(context.Context) error                             { return nil }
func (selectOneDriver) Close() error                                           { return nil }
func (selectOneDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{Rows: []result.Row{{}}}, nil
}
func (selectOneDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (selectOneDriver) Dialect() driver.Dialect { return driver.DialectSQLite }

func TestHarnessAcceptsValidEngine(t *testing.T) {
	dbengine.Register(dbengine.Registration{
		ID: "harness-fake", DisplayName: "Harness Fake", Dialect: dbengine.DialectSQLite,
		NewDriver: func() driver.Driver { return selectOneDriver{} },
	})
	enginetest.RunCapabilityContract(t, "harness-fake")
	enginetest.RunConnectionContract(t, "harness-fake", dbengine.ConnectionConfig{DSN: "ignored", Driver: "harness-fake"})
}
