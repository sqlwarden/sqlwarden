package sqlite

import (
	"context"
	"errors"
	"strings"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/parser"
)

var _ parser.Parser = (*sqliteDriver)(nil)

func (d *sqliteDriver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	stmts, spans, err := parseSQLite(ctx, req.SQL)
	if err != nil {
		return parser.Result{}, err
	}
	return parser.Result{
		AST:            parser.NewOpaqueAST(stmts),
		Statements:     spans,
		StatementCount: len(stmts),
	}, nil
}

// parseSQLite parses sql with rqlite/sql, normalizing a parse error into a
// *parser.SyntaxError and pairing each parsed statement with its byte span in
// the original text. Context cancellation is checked before and after the
// parse; the parser itself is not context-aware.
func parseSQLite(ctx context.Context, sql string) ([]rqlitesql.Statement, []parser.Statement, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	stmts, err := rqlitesql.NewParser(strings.NewReader(sql)).ParseStatements()
	if err != nil {
		var perr *rqlitesql.Error
		if errors.As(err, &perr) {
			offset := parser.ClampOffset(sql, perr.Pos.Offset)
			line, column := parser.Position(sql, offset)
			return nil, nil, &parser.SyntaxError{
				Message: perr.Msg,
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

	spans := sqliteStatementSpans(sql)
	if len(spans) != len(stmts) {
		// Span/AST count disagreement (comment-only trailing segment, unusual
		// delimiter placement): fall back to one full-input span per statement,
		// mirroring the mysql parser's fallback.
		spans = make([]parser.Statement, len(stmts))
		for i := range spans {
			spans[i] = parser.Statement{StartOffset: 0, EndOffset: len(sql)}
		}
	}
	return stmts, spans, nil
}

// sqliteStatementSpans splits sql into [start, end) byte spans on top-level
// semicolons (depth 0, outside string/quoted-ident literals — the scanner
// handles those). A leading run of comments is not counted as the statement
// start; the trailing ';' is excluded from the span.
func sqliteStatementSpans(sql string) []parser.Statement {
	s := rqlitesql.NewScanner(strings.NewReader(sql))
	var spans []parser.Statement
	depth := 0
	start := 0
	started := false
	for {
		pos, tok, _ := s.Scan()
		switch tok {
		case rqlitesql.EOF:
			if started {
				spans = append(spans, parser.Statement{StartOffset: start, EndOffset: len(sql)})
			}
			return spans
		case rqlitesql.LP:
			depth++
		case rqlitesql.RP:
			if depth > 0 {
				depth--
			}
		case rqlitesql.SEMI:
			if depth == 0 && started {
				spans = append(spans, parser.Statement{StartOffset: start, EndOffset: pos.Offset})
				started = false
				continue
			}
		}
		if !started && tok != rqlitesql.COMMENT && tok != rqlitesql.SEMI {
			start = pos.Offset
			started = true
		}
	}
}
