package oracle

import (
	"context"
	"errors"

	"github.com/bytebase/omni/oracle"
	oracleparser "github.com/bytebase/omni/oracle/parser"

	"github.com/sqlwarden/internal/engine/parser"
)

var _ parser.Parser = (*oracleDriver)(nil)

func (d *oracleDriver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	statements, spans, err := parseOracle(ctx, req.SQL)
	if err != nil {
		return parser.Result{}, err
	}
	return parser.Result{
		AST:            parser.NewOpaqueAST(statements),
		Statements:     spans,
		StatementCount: len(statements),
	}, nil
}

// parseOracle parses a full script, returning the omni statements alongside
// SQLWarden statement spans. A syntax error is returned as *parser.SyntaxError.
func parseOracle(ctx context.Context, sql string) ([]oracle.Statement, []parser.Statement, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	parsed, err := oracle.Parse(sql)
	if err != nil {
		var parseErr *oracleparser.ParseError
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

	statements := make([]oracle.Statement, 0, len(parsed))
	spans := make([]parser.Statement, 0, len(parsed))
	for _, stmt := range parsed {
		if stmt.Empty() {
			continue
		}
		start, end := stmt.ByteStart, stmt.ByteEnd
		if start < 0 {
			start = 0
		}
		if end > len(sql) {
			end = len(sql)
		}
		for start < end && (sql[start] == ';' || sql[start] == '\n' || sql[start] == '\r' || sql[start] == '\t' || sql[start] == ' ') {
			start++
		}
		for end > start && (sql[end-1] == ';' || sql[end-1] == '\n' || sql[end-1] == '\r' || sql[end-1] == '\t' || sql[end-1] == ' ') {
			end--
		}
		statements = append(statements, stmt)
		spans = append(spans, parser.Statement{StartOffset: start, EndOffset: end})
	}
	return statements, spans, nil
}
