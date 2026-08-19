package safety

import (
	"context"
	"strings"
)

type heuristic struct{}

// NewHeuristic returns the conservative, dialect-agnostic WHERE-clause
// checker used as a fallback for dialects without a registered Checker
// (currently sqlite). Best-effort, not exact: a WHERE keyword inside a
// subquery in the SET clause of an UPDATE is misread as satisfying the
// check. This mirrors the accuracy tradeoff classifier.heuristic already
// makes for the same dialects.
func NewHeuristic() Checker { return heuristic{} }

func (heuristic) Check(_ context.Context, req Request) (Result, error) {
	var statements []UnsafeStatement
	offset := 0
	for _, segment := range splitStatements(req.SQL) {
		start := offset
		end := offset + len(segment)
		offset = end + 1 // account for the ';' delimiter consumed by splitStatements

		trimmed := strings.TrimSpace(segment)
		if trimmed == "" {
			continue
		}
		upper := strings.ToUpper(trimmed)
		isUpdate := strings.HasPrefix(upper, "UPDATE ") || strings.HasPrefix(upper, "UPDATE\t")
		isDelete := strings.HasPrefix(upper, "DELETE ") || strings.HasPrefix(upper, "DELETE\t")
		if !isUpdate && !isDelete {
			continue
		}
		if hasTopLevelWhere(upper) {
			continue
		}
		statements = append(statements, UnsafeStatement{
			Kind:        KindUnsafeMissingWhere,
			StartOffset: start,
			EndOffset:   end,
		})
	}
	return Result{Unsafe: len(statements) > 0, Statements: statements, Source: "heuristic"}, nil
}

// splitStatements splits sql on ';' outside single- and double-quoted
// strings. SQLite has no custom-delimiter feature like MySQL's DELIMITER, so
// this stays simple.
func splitStatements(sql string) []string {
	var segments []string
	var quote byte
	start := 0
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == ';':
			segments = append(segments, sql[start:i])
			start = i + 1
		}
	}
	segments = append(segments, sql[start:])
	return segments
}

// hasTopLevelWhere scans an already-uppercased statement for a WHERE keyword
// outside quoted strings.
func hasTopLevelWhere(upper string) bool {
	var quote byte
	for i := 0; i < len(upper); i++ {
		c := upper[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == 'W' && strings.HasPrefix(upper[i:], "WHERE") {
			before := i == 0 || !isIdentByte(upper[i-1])
			afterIdx := i + len("WHERE")
			after := afterIdx >= len(upper) || !isIdentByte(upper[afterIdx])
			if before && after {
				return true
			}
		}
	}
	return false
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
