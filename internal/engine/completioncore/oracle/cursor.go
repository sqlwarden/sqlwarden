package oracle

import (
	"strings"
	"unicode"

	oracleparser "github.com/bytebase/omni/oracle/parser"
)

// CursorWord returns a whole identifier replacement range, including quotes
// and any suffix after the cursor. Suppressed positions are literals/comments.
func CursorWord(sql string, cursor int) (start, end int, prefix string, suppressed bool) {
	start, end = cursor, cursor
	lexer := oracleparser.NewLexer(sql)
	previous := 0
	for {
		token := lexer.NextToken()
		if cursor <= token.Loc || token.Type == 0 && token.Loc == token.End {
			return start, end, "", commentOpen(sql[previous:cursor])
		}
		if cursor > token.Loc && cursor <= token.End {
			raw := sql[token.Loc:token.End]
			upper := strings.ToUpper(raw)
			if strings.HasPrefix(raw, "\"") {
				prefix = sql[token.Loc+1 : cursor]
				if cursor == token.End && lexer.Err == nil {
					prefix = strings.TrimSuffix(prefix, "\"")
				}
				return token.Loc, token.End, strings.ReplaceAll(prefix, "\"\"", "\""), false
			}
			if strings.HasPrefix(raw, "'") || strings.HasPrefix(upper, "N'") || strings.HasPrefix(upper, "Q'") || strings.HasPrefix(raw, "/*") {
				return start, end, "", cursor < token.End || lexer.Err != nil || strings.HasPrefix(raw, "/*")
			}
			if raw != "" && (unicode.IsLetter([]rune(raw)[0]) || raw[0] == '_') {
				return token.Loc, token.End, sql[token.Loc:cursor], false
			}
			return start, end, "", false
		}
		if token.Type == 0 {
			return start, end, "", commentOpen(sql[previous:cursor])
		}
		previous = token.End
	}
}

// The lexer skips ordinary comments; only those skipped spans reach here.
func commentOpen(gap string) bool {
	for i := 0; i < len(gap); {
		if strings.HasPrefix(gap[i:], "--") {
			newline := strings.IndexByte(gap[i:], '\n')
			if newline < 0 {
				return true
			}
			i += newline + 1
		} else if strings.HasPrefix(gap[i:], "/*") {
			depth := 1
			i += 2
			for i < len(gap) && depth > 0 {
				switch {
				case strings.HasPrefix(gap[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(gap[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
			if depth > 0 {
				return true
			}
		} else {
			i++
		}
	}
	return false
}
