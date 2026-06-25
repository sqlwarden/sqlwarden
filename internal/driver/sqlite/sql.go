package sqlite

import (
	"github.com/sqlwarden/internal/dbengine/sqlquery"
	"github.com/sqlwarden/internal/dbengine/sqlquery/gosqlx"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	sqlquery.Register(driver.DialectSQLite, sqlquery.StaticProvider{
		ClassifyCapability: gosqlx.NewClassifier(),
		ParseCapability:    gosqlx.NewParser(),
		RewriteCapability:  gosqlx.NewRewriter(),
	})
}
