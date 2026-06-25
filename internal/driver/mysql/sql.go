package mysql

import (
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	classifier.Register(driver.DialectMySQL, gosqlx.NewClassifier(driver.DialectMySQL))
	parser.Register(driver.DialectMySQL, gosqlx.NewParser(driver.DialectMySQL))
	rewriter.Register(driver.DialectMySQL, gosqlx.NewRewriter(driver.DialectMySQL))
}
