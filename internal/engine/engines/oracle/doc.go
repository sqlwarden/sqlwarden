// Package oracle registers the Oracle database engine implementation. It wraps
// github.com/sijms/go-ora/v2 for connectivity and github.com/bytebase/omni/oracle
// for SQL parsing, classification, safety analysis, and completion. It provides
// the full Postgres-tier capability surface except explain.Explainer: Oracle
// EXPLAIN requires a two-statement script, which the single-string Explainer
// contract cannot express (tracked in SQLW-107).
//
// The supported baseline is Oracle 12.1+: row-limiting clauses
// (FETCH FIRST ... ROWS ONLY / OFFSET) are used for bounded previews and counts,
// and 11g (which requires ROWNUM predicates) is not supported.
package oracle
