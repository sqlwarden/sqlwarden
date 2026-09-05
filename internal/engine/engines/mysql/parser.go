package mysql

import (
	"context"
	"errors"

	"github.com/bytebase/omni/mysql/ast"
	omniparser "github.com/bytebase/omni/mysql/parser"

	"github.com/sqlwarden/internal/engine/parser"
)

var _ parser.Parser = (*Driver)(nil)

func (d *Driver) Parse(ctx context.Context, req parser.Request) (parser.Result, error) {
	tree, spans, err := parseMySQL(ctx, req.SQL)
	if err != nil {
		return parser.Result{}, err
	}
	return parser.Result{
		AST:            parser.NewOpaqueAST(tree),
		Statements:     spans,
		StatementCount: tree.Len(),
	}, nil
}

func parseMySQL(ctx context.Context, sql string) (*ast.List, []parser.Statement, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	tree, err := omniparser.Parse(sql)
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

	segments := omniparser.Split(sql)
	spans := make([]parser.Statement, 0, len(segments))
	for _, segment := range segments {
		if segment.Empty() {
			continue
		}
		spans = append(spans, parser.Statement{
			StartOffset: segment.ByteStart,
			EndOffset:   segment.ByteEnd,
		})
	}
	if tree.Len() != len(spans) {
		// Executable version comments can expand one source segment into
		// multiple AST statements. Their exact textual boundary is the shared
		// segment, so preserve one result entry per AST statement with that
		// common range.
		spans = spans[:0]
		for _, segment := range segments {
			segmentTree, segmentErr := omniparser.Parse(segment.Text)
			if segmentErr != nil {
				spans = nil
				break
			}
			for range segmentTree.Len() {
				spans = append(spans, parser.Statement{
					StartOffset: segment.ByteStart,
					EndOffset:   segment.ByteEnd,
				})
			}
		}
		if len(spans) != tree.Len() {
			spans = make([]parser.Statement, tree.Len())
			for i := range spans {
				spans[i] = parser.Statement{StartOffset: 0, EndOffset: len(sql)}
			}
		}
	}
	return tree, spans, nil
}
