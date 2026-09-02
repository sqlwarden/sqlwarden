// Package oracle registers the Oracle database engine implementation. It wraps
// github.com/sijms/go-ora/v2 for connectivity and github.com/bytebase/omni/oracle
// for SQL parsing, classification, safety analysis, and completion. It provides
// the full Postgres-tier capability surface. Oracle EXPLAIN is a two-statement
// script (EXPLAIN PLAN FOR ... then a DBMS_XPLAN query), expressed through the
// explain.Plan Setup/Statement/Teardown fields.
//
// The supported baseline is Oracle 12.1+: row-limiting clauses
// (FETCH FIRST ... ROWS ONLY / OFFSET) are used for bounded previews and counts,
// and 11g (which requires ROWNUM predicates) is not supported.
//
// oracle_integration_test.go carries the //go:build integration tag: it starts a
// gvenzl/oracle-free container via testcontainers and exercises the driver,
// inspector, DDL executor and transaction controller against a live instance. It
// is excluded from the default `go test ./...` run; execute it with
// `go test -tags integration ./internal/engine/engines/oracle/...` and a running
// Docker daemon.
package oracle
