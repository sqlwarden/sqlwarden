package postgres

import (
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/gosqlx"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	classifier.Register(driver.DialectPostgres, gosqlx.NewClassifier(driver.DialectPostgres))
	parser.Register(driver.DialectPostgres, gosqlx.NewParser(driver.DialectPostgres))
	rewriter.Register(driver.DialectPostgres, gosqlx.NewRewriter(driver.DialectPostgres))
}
