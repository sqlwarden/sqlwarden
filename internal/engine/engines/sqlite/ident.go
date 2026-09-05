package sqlite

import (
	"strings"

	rqlitesql "github.com/rqlite/sql"
)

// sqliteQuoteIdent double-quotes an identifier, escaping embedded quotes. The
// folding rules must match the frontend sqlite dialect.
func sqliteQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// sqliteQualify renders namespace.name with each part quoted. An empty
// namespace yields the bare quoted name.
func sqliteQualify(namespace, name string) string {
	if namespace == "" {
		return sqliteQuoteIdent(name)
	}
	return sqliteQuoteIdent(namespace) + "." + sqliteQuoteIdent(name)
}

// sqliteIsBareIdent reports whether s can be emitted unquoted: a
// [A-Za-z_][A-Za-z0-9_]* token that is not a reserved keyword.
func sqliteIsBareIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
		case c >= '0' && c <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return rqlitesql.Lookup(strings.ToUpper(s)) == rqlitesql.IDENT
}
