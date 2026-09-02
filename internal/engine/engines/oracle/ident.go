package oracle

import "strings"

// oracleQuoteIdent wraps an identifier in double quotes, doubling any embedded
// double quote. SQLWarden always quotes identifiers, so the upper-cased names
// returned by the ALL_* data-dictionary views round-trip unchanged.
func oracleQuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// oracleQualified renders "SCHEMA"."NAME", or just "NAME" when schema is empty.
func oracleQualified(schema, name string) string {
	if schema == "" {
		return oracleQuoteIdent(name)
	}
	return oracleQuoteIdent(schema) + "." + oracleQuoteIdent(name)
}
