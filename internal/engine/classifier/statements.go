package classifier

import "strings"

// CountStatements returns the number of non-empty top-level SQL statements in
// text, splitting on semicolons that are not inside string literals or
// comments. It is a conservative fallback for engines whose Classifier
// cannot report Result.StatementCount (which reads 0 for "unknown," not
// "one") — typically engines with no AST parser available.
func CountStatements(text string) int {
	return len(statementSpans(text))
}

type lexState int

const (
	stateNormal lexState = iota
	stateSingleQuote
	stateDoubleQuote
	stateLineComment
	stateBlockComment
)

// statementSpans splits text into [start, end) byte spans on top-level
// semicolons, skipping empty spans. Mirrors the frontend's
// sqlStatementSpans/countSqlStatements lexer so both layers treat the same
// input identically.
func statementSpans(text string) [][2]int {
	state := stateNormal
	var semis []int

	for i := 0; i < len(text); i++ {
		c := text[i]
		var n byte
		if i+1 < len(text) {
			n = text[i+1]
		}
		switch state {
		case stateNormal:
			switch {
			case c == '\'':
				state = stateSingleQuote
			case c == '"':
				state = stateDoubleQuote
			case c == '-' && n == '-':
				state = stateLineComment
				i++
			case c == '/' && n == '*':
				state = stateBlockComment
				i++
			case c == ';':
				semis = append(semis, i)
			}
		case stateSingleQuote:
			if c == '\'' && n == '\'' {
				i++
			} else if c == '\'' {
				state = stateNormal
			}
		case stateDoubleQuote:
			if c == '"' && n == '"' {
				i++
			} else if c == '"' {
				state = stateNormal
			}
		case stateLineComment:
			if c == '\n' {
				state = stateNormal
			}
		case stateBlockComment:
			if c == '*' && n == '/' {
				state = stateNormal
				i++
			}
		}
	}

	var spans [][2]int
	start := 0
	if len(semis) == 0 {
		if strings.TrimSpace(text) != "" {
			spans = append(spans, [2]int{0, len(text)})
		}
		return spans
	}
	for _, semi := range semis {
		if strings.TrimSpace(text[start:semi+1]) != "" {
			spans = append(spans, [2]int{start, semi + 1})
		}
		nextStart := semi + 1
		for nextStart < len(text) && isSQLSpace(text[nextStart]) {
			nextStart++
		}
		start = nextStart
	}
	if strings.TrimSpace(text[start:]) != "" {
		spans = append(spans, [2]int{start, len(text)})
	}
	return spans
}

func isSQLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
