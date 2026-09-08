package sqlserver

import (
	"context"
	"errors"

	omnimssql "github.com/bytebase/omni/mssql"
	"github.com/bytebase/omni/mssql/ast"
	omniparser "github.com/bytebase/omni/mssql/parser"

	"github.com/sqlwarden/internal/engine/parser"
)

var _ parser.Parser = (*Driver)(nil)

func (d *Driver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	statements, spans, err := parseSQLServer(ctx, req.SQL)
	if err != nil {
		return parser.Result{}, err
	}
	return parser.Result{
		AST:            parser.NewOpaqueAST(statements),
		Statements:     spans,
		StatementCount: len(statements),
	}, nil
}

// parseSQLServer parses a full script, excluding GO batch separators (a
// sqlcmd/SSMS client convention, not valid T-SQL) from both the returned
// statement list and the span list — forwarding "GO" to the server as a
// statement is a syntax error.
func parseSQLServer(ctx context.Context, sql string) ([]omnimssql.Statement, []parser.Statement, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	statements, err := omnimssql.Parse(sql)
	if err != nil {
		var parseErr *omniparser.ParseError
		if errors.As(err, &parseErr) {
			offset := parser.ClampOffset(sql, parseErr.Position)
			line, column := parser.Position(sql, offset)
			return nil, nil, &parser.SyntaxError{
				Message: parseErr.Message,
				Offset:  offset,
				Line:    line,
				Column:  column,
			}
		}
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	filtered := make([]omnimssql.Statement, 0, len(statements))
	spans := make([]parser.Statement, 0, len(statements))
	for _, statement := range statements {
		if statement.AST == nil {
			continue
		}
		if _, isGo := statement.AST.(*ast.GoStmt); isGo {
			continue
		}
		start, end := statement.ByteStart, statement.ByteEnd
		if end > start && end <= len(sql) && sql[end-1] == ';' {
			end--
		}
		filtered = append(filtered, statement)
		spans = append(spans, parser.Statement{StartOffset: start, EndOffset: end})
	}
	return filtered, spans, nil
}
