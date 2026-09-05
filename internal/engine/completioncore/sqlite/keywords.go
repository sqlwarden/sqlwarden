// Package sqlite adapts rqlite/sql's scanner to SQLWarden's dialect-neutral
// completion boundary. rqlite/sql exposes no ANTLR candidate collector, so
// cursor intent is derived from a backward token scan and every name is
// resolved from SQLWarden's own metadata index via
// completioncore.MetadataResolver. No completer opens a live connection.
package sqlite

import "sort"

// keywords is the SQLite statement/clause keyword set surfaced by completion.
// Sourced from rqlite/sql's token table plus the common clause words.
var keywords = []string{
	"ABORT", "ACTION", "ADD", "AFTER", "ALL", "ALTER", "ALWAYS", "ANALYZE",
	"AND", "AS", "ASC", "ATTACH", "AUTOINCREMENT", "BEFORE", "BEGIN", "BETWEEN",
	"BY", "CASCADE", "CASE", "CAST", "CHECK", "COLLATE", "COLUMN", "COMMIT",
	"CONFLICT", "CONSTRAINT", "CREATE", "CROSS", "CURRENT", "CURRENT_DATE",
	"CURRENT_TIME", "CURRENT_TIMESTAMP", "DATABASE", "DEFAULT", "DEFERRABLE",
	"DEFERRED", "DELETE", "DESC", "DETACH", "DISTINCT", "DO", "DROP", "EACH",
	"ELSE", "END", "ESCAPE", "EXCEPT", "EXCLUDE", "EXCLUSIVE", "EXISTS",
	"EXPLAIN", "FAIL", "FILTER", "FIRST", "FOLLOWING", "FOR", "FOREIGN", "FROM",
	"FULL", "GENERATED", "GLOB", "GROUP", "GROUPS", "HAVING", "IF", "IGNORE",
	"IMMEDIATE", "IN", "INDEX", "INDEXED", "INITIALLY", "INNER", "INSERT",
	"INSTEAD", "INTERSECT", "INTO", "IS", "ISNULL", "JOIN", "KEY", "LAST",
	"LEFT", "LIKE", "LIMIT", "MATCH", "MATERIALIZED", "NATURAL", "NO", "NOT",
	"NOTHING", "NOTNULL", "NULL", "NULLS", "OF", "OFFSET", "ON", "OR", "ORDER",
	"OTHERS", "OUTER", "OVER", "PARTITION", "PLAN", "PRAGMA", "PRECEDING",
	"PRIMARY", "QUERY", "RAISE", "RANGE", "RECURSIVE", "REFERENCES", "REGEXP",
	"REINDEX", "RELEASE", "RENAME", "REPLACE", "RESTRICT", "RETURNING", "RIGHT",
	"ROLLBACK", "ROW", "ROWS", "SAVEPOINT", "SELECT", "SET", "TABLE", "TEMP",
	"TEMPORARY", "THEN", "TIES", "TO", "TRANSACTION", "TRIGGER", "UNBOUNDED",
	"UNION", "UNIQUE", "UPDATE", "USING", "VACUUM", "VALUES", "VIEW", "VIRTUAL",
	"WHEN", "WHERE", "WINDOW", "WITH", "WITHOUT",
}

// functions is the built-in scalar/aggregate function set for the vocabulary
// endpoint (completer.CompletionVocabulary).
var functions = []string{
	"abs", "changes", "char", "coalesce", "concat", "concat_ws", "format",
	"glob", "hex", "ifnull", "iif", "instr", "last_insert_rowid", "length",
	"like", "likelihood", "likely", "lower", "ltrim", "max", "min", "nullif",
	"printf", "quote", "random", "randomblob", "replace", "round", "rtrim",
	"sign", "soundex", "sqlite_version", "substr", "substring", "total_changes",
	"trim", "typeof", "unhex", "unicode", "unlikely", "upper", "zeroblob",
	"avg", "count", "group_concat", "string_agg", "sum", "total",
	"date", "time", "datetime", "julianday", "unixepoch", "strftime", "timediff",
	"json", "json_array", "json_extract", "json_object", "json_type",
	"json_valid", "json_quote", "json_group_array", "json_group_object",
	"row_number", "rank", "dense_rank", "percent_rank", "cume_dist", "ntile",
	"lag", "lead", "first_value", "last_value", "nth_value",
}

// typeAffinities is the SQLite column-type vocabulary for DDL/type positions.
var typeAffinities = []string{
	"INTEGER", "INT", "TINYINT", "SMALLINT", "MEDIUMINT", "BIGINT",
	"UNSIGNED BIG INT", "INT2", "INT8",
	"TEXT", "CHARACTER", "VARCHAR", "NCHAR", "NATIVE CHARACTER", "NVARCHAR",
	"CLOB", "REAL", "DOUBLE", "DOUBLE PRECISION", "FLOAT",
	"NUMERIC", "DECIMAL", "BOOLEAN", "DATE", "DATETIME", "BLOB",
}

// Keywords returns a sorted copy of the SQLite keyword set.
func Keywords() []string {
	out := append([]string(nil), keywords...)
	sort.Strings(out)
	return out
}

// Functions returns a sorted copy of the built-in function names.
func Functions() []string {
	out := append([]string(nil), functions...)
	sort.Strings(out)
	return out
}

// TypeAffinities returns a sorted copy of the column-type vocabulary.
func TypeAffinities() []string {
	out := append([]string(nil), typeAffinities...)
	sort.Strings(out)
	return out
}
