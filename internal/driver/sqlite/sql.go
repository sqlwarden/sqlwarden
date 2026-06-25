package sqlite

import (
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	classifier.Register(driver.DialectSQLite, gosqlx.NewClassifier(driver.DialectSQLite))
	parser.Register(driver.DialectSQLite, gosqlx.NewParser(driver.DialectSQLite))
	rewriter.Register(driver.DialectSQLite, gosqlx.NewRewriter(driver.DialectSQLite))
}
