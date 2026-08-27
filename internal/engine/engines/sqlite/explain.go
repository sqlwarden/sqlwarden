package sqlite

import (
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*sqliteDriver)(nil)

var sqliteExplainSpec = explain.Spec{SupportsAnalyze: false}

func (*sqliteDriver) ExplainSpec() explain.Spec { return sqliteExplainSpec }

// Explain wraps sql with SQLite's EXPLAIN QUERY PLAN. SQLite has no ANALYZE
// variant that reports execution timing per plan node.
func (*sqliteDriver) Explain(sql string, mode explain.Mode) (string, error) {
	if err := validateExplainable(sql); err != nil {
		return "", err
	}
	switch mode {
	case explain.ModePlain:
		return fmt.Sprintf("EXPLAIN QUERY PLAN %s", sql), nil
	case explain.ModeAnalyze:
		return "", explain.ErrAnalyzeUnsupported
	default:
		return "", explain.ErrUnsupported
	}
}

// validateExplainable checks that sql is exactly one statement and not
// already an EXPLAIN, returning explain.ErrMultipleStatements or
// explain.ErrAlreadyExplained respectively. SQLite has no AST parser
// available in this codebase's dependencies (see classifier package), so
// this is lexical best-effort rather than a real parse.
func validateExplainable(sql string) error {
	if classifier.CountStatements(sql) != 1 {
		return explain.ErrMultipleStatements
	}
	if isAlreadyExplained(sql) {
		return explain.ErrAlreadyExplained
	}
	return nil
}

// isAlreadyExplained reports whether sql's leading keyword is EXPLAIN. It
// skips leading whitespace and comments, then matches the EXPLAIN keyword on
// a word boundary.
func isAlreadyExplained(sql string) bool {
	i := 0
	for i < len(sql) {
		switch {
		case isSQLSpace(sql[i]):
			i++
		case strings.HasPrefix(sql[i:], "--"):
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				return false
			}
			i += end + 1
		case strings.HasPrefix(sql[i:], "/*"):
			end := strings.Index(sql[i:], "*/")
			if end < 0 {
				return false
			}
			i += end + 2
		default:
			rest := sql[i:]
			const keyword = "explain"
			if len(rest) < len(keyword) || !strings.EqualFold(rest[:len(keyword)], keyword) {
				return false
			}
			if len(rest) == len(keyword) {
				return true
			}
			return !isIdentChar(rest[len(keyword)])
		}
	}
	return false
}

func isSQLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
