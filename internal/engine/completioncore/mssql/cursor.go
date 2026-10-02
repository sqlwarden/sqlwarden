package mssql

import "strings"

// CursorWord returns the replacement range and typed prefix at cursor. The
// range starts at an opening bracket so a bracketed name is replaced whole.
// Suppressed positions are inside a comment or string literal.
func CursorWord(sql string, cursor int) (start, end int, prefix string, suppressed bool) {
	cursor = max(0, min(cursor, len(sql)))
	suppressed, quotedStart, closing := scanCursorContext(sql, cursor)
	if quotedStart >= 0 {
		prefix = sql[quotedStart+1 : cursor]
		delimiter := string(closing)
		return quotedStart, cursor, strings.ReplaceAll(prefix, delimiter+delimiter, delimiter), false
	}

	start = cursor
	for start > 0 && isIdentifierByte(sql[start-1]) {
		start--
	}
	prefix = sql[start:cursor]
	if start > 0 && sql[start-1] == '[' {
		start--
	}
	return start, cursor, prefix, suppressed
}

func isIdentifierByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '_' || c == '$' || c == '#' || c == '@' || c >= 0x80
}

// scanCursorContext scans sql up to pos. Bracketed and double-quoted identifiers
// are skipped so their contents never open a comment or string.
func scanCursorContext(sql string, pos int) (suppressed bool, quotedStart int, closing byte) {
	for i := 0; i < pos; {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			newline := strings.IndexByte(sql[i:], '\n')
			if newline < 0 || i+newline >= pos {
				return true, -1, 0
			}
			i += newline + 1
		case strings.HasPrefix(sql[i:], "/*"):
			closing := strings.Index(sql[i+2:], "*/")
			if closing < 0 || i+2+closing+2 > pos {
				return true, -1, 0
			}
			i += 2 + closing + 2
		case sql[i] == '\'':
			next, closed := skipQuoted(sql, i, '\'')
			if !closed || next > pos {
				return true, -1, 0
			}
			i = next
		case sql[i] == '[' || sql[i] == '"':
			closing = ']'
			if sql[i] == '"' {
				closing = '"'
			}
			next, closed := skipQuoted(sql, i, closing)
			if !closed || next > pos {
				return false, i, closing
			}
			i = next
		default:
			i++
		}
	}
	return false, -1, 0
}

// skipQuoted returns the offset after the delimiter closing the literal or
// quoted identifier that opens at i; a doubled delimiter is an escape.
func skipQuoted(sql string, i int, closing byte) (int, bool) {
	for j := i + 1; j < len(sql); j++ {
		if sql[j] != closing {
			continue
		}
		if j+1 < len(sql) && sql[j+1] == closing {
			j++
			continue
		}
		return j + 1, true
	}
	return len(sql), false
}
