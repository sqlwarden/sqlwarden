package sqlite

import (
	"context"
	"errors"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/safety"
)

var _ safety.Checker = (*sqliteDriver)(nil)

func (d *sqliteDriver) Check(ctx context.Context, req safety.Request) (safety.Result, error) {
	stmts, spans, err := parseSQLite(ctx, req.SQL)
	if err != nil {
		var se *parser.SyntaxError
		if errors.As(err, &se) {
			// rqlite/sql's grammar is narrower than the SQLite runtime's (it
			// rejects VACUUM, ATTACH/DETACH, data-modifying CTEs, and more), so a
			// parse failure is not proof the statement is inert. Fall back to the
			// lexical heuristic to keep the missing-WHERE confirmation rail.
			return safety.NewHeuristic().Check(ctx, req)
		}
		return safety.Result{}, err
	}

	var unsafe []safety.UnsafeStatement
	for i, stmt := range stmts {
		if sqliteStatementMissingWhere(stmt) {
			unsafe = append(unsafe, safety.UnsafeStatement{
				Kind:        safety.KindUnsafeMissingWhere,
				StartOffset: spans[i].StartOffset,
				EndOffset:   spans[i].EndOffset,
			})
		}
	}
	return safety.Result{Unsafe: len(unsafe) > 0, Statements: unsafe, Source: "rqlite"}, nil
}

// sqliteStatementMissingWhere reports an UPDATE/DELETE with no bound on the rows
// it touches — the pattern that silently rewrites or removes every row. DELETE
// admits a LIMIT bound; rqlite/sql's UpdateStatement has no LIMIT field (SQLite
// only parses UPDATE ... LIMIT under SQLITE_ENABLE_UPDATE_DELETE_LIMIT), so an
// UPDATE is judged on its WHERE clause alone.
func sqliteStatementMissingWhere(stmt rqlitesql.Statement) bool {
	switch s := stmt.(type) {
	case *rqlitesql.UpdateStatement:
		return s.WhereExpr == nil
	case *rqlitesql.DeleteStatement:
		return s.WhereExpr == nil && s.LimitExpr == nil
	default:
		return false
	}
}
