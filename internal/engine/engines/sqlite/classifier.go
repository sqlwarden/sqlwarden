package sqlite

import (
	"context"
	"errors"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

var _ classifier.Classifier = (*sqliteDriver)(nil)

func (d *sqliteDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	stmts, _, err := parseSQLite(ctx, req.SQL)
	if err != nil {
		var se *parser.SyntaxError
		if errors.As(err, &se) {
			return classifier.Result{Kind: classifier.KindUnknown, Source: "rqlite"}, nil
		}
		return classifier.Result{}, err
	}

	kind := classifier.KindUnknown
	for i, stmt := range stmts {
		sk := classifySQLiteStatement(stmt)
		if i == 0 {
			kind = sk
		} else {
			kind = combineSQLiteKinds(kind, sk)
		}
	}
	return classifier.Result{Kind: kind, Source: "rqlite", StatementCount: len(stmts)}, nil
}

func classifySQLiteStatement(stmt rqlitesql.Statement) classifier.Kind {
	switch s := stmt.(type) {
	case *rqlitesql.SelectStatement:
		return classifySQLiteSelect(s)
	case *rqlitesql.InsertStatement, *rqlitesql.UpdateStatement, *rqlitesql.DeleteStatement:
		return classifier.KindDML
	case *rqlitesql.CreateTableStatement, *rqlitesql.CreateViewStatement,
		*rqlitesql.CreateIndexStatement, *rqlitesql.CreateTriggerStatement,
		*rqlitesql.CreateVirtualTableStatement, *rqlitesql.AlterTableStatement,
		*rqlitesql.DropTableStatement, *rqlitesql.DropViewStatement,
		*rqlitesql.DropIndexStatement, *rqlitesql.DropTriggerStatement,
		*rqlitesql.ReindexStatement:
		return classifier.KindDDL
	case *rqlitesql.ExplainStatement:
		if s.Stmt == nil {
			return classifier.KindUnknown
		}
		return classifySQLiteStatement(s.Stmt)
	default:
		// AnalyzeStatement, PragmaStatement, Begin/Commit/Rollback/Savepoint/
		// Release, and anything unrecognized: the RBAC layer treats Unknown as
		// maximally privileged, which is the safe default for statements that
		// can mutate schema or data. VACUUM, ATTACH, and DETACH never reach
		// here — rqlite/sql cannot parse them, so they surface as a syntax
		// error and are classified Unknown above.
		return classifier.KindUnknown
	}
}

// classifySQLiteSelect returns KindDQL for a plain read, downgrading to
// KindUnknown if the statement embeds a mutation via a CTE (WITH ... DELETE/
// INSERT/UPDATE ... RETURNING). SQLite has no SELECT ... FOR UPDATE.
func classifySQLiteSelect(s *rqlitesql.SelectStatement) classifier.Kind {
	if s == nil {
		return classifier.KindUnknown
	}
	mutating := false
	_, _ = rqlitesql.Walk(walkFunc(func(n rqlitesql.Node) bool {
		switch n.(type) {
		case *rqlitesql.InsertStatement, *rqlitesql.UpdateStatement, *rqlitesql.DeleteStatement:
			mutating = true
			return false
		}
		return !mutating
	}), s)
	if mutating {
		return classifier.KindUnknown
	}
	return classifier.KindDQL
}

func combineSQLiteKinds(left, right classifier.Kind) classifier.Kind {
	if left == classifier.KindUnknown || right == classifier.KindUnknown {
		return classifier.KindUnknown
	}
	if left == classifier.KindDQL {
		return right
	}
	if right == classifier.KindDQL || left == right {
		return left
	}
	return classifier.KindUnknown
}

// walkFunc adapts a func to rqlite/sql's Visitor interface. Returning false
// from the func stops descent into the current node's children.
type walkFunc func(rqlitesql.Node) bool

func (f walkFunc) Visit(n rqlitesql.Node) (rqlitesql.Visitor, rqlitesql.Node, error) {
	if f(n) {
		return f, n, nil
	}
	return nil, n, nil
}

func (f walkFunc) VisitEnd(n rqlitesql.Node) (rqlitesql.Node, error) { return n, nil }
