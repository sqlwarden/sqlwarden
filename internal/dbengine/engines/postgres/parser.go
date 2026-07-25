package postgres

import (
	"context"
	"errors"

	omnpg "github.com/bytebase/omni/pg"
	omniparser "github.com/bytebase/omni/pg/parser"

	"github.com/sqlwarden/internal/dbengine/parser"
)

var _ parser.Parser = (*postgresDriver)(nil)

func (d *postgresDriver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	statements, spans, err := parsePostgres(ctx, req.SQL)
	if err != nil {
		return parser.Result{}, err
	}
	return parser.Result{
		AST:            parser.NewOpaqueAST(statements),
		Statements:     spans,
		StatementCount: len(statements),
	}, nil
}

func parsePostgres(ctx context.Context, sql string) ([]omnpg.Statement, []parser.Statement, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	statements, err := omnpg.Parse(sql)
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

	filtered := make([]omnpg.Statement, 0, len(statements))
	spans := make([]parser.Statement, 0, len(statements))
	for _, statement := range statements {
		if statement.AST == nil {
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
