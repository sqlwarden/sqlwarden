package mssql

import (
	"regexp"
	"strings"

	mssqlparser "github.com/bytebase/omni/mssql/parser"
)

var regularIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$#@]*$`)

// QuoteIdentifier returns name bare when it is a regular identifier that is
// not a reserved word, and bracket-quoted otherwise.
func QuoteIdentifier(name string) string {
	if regularIdentifier.MatchString(name) {
		if tokens := mssqlparser.Tokenize(name); len(tokens) == 1 && mssqlparser.IsIdentTokenType(tokens[0].Type) {
			return name
		}
	}
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}
