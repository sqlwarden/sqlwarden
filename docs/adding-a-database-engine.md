# Adding a Database Engine

This guide is the checklist for bringing a new target database to SQLWarden at
full capability parity, derived from the PostgreSQL/MySQL baseline and the Oracle
implementation (SQLW-103). Follow it top to bottom; every section that a real
engine skips should be a deliberate, documented choice, not an oversight.

The architecture context for this guide is `docs/sqlwarden-architecture.md`
(sections *Query Execution And Live Sessions*, *Schema Introspection*, and
*Implementation Invariants*). Read that first.

## Design rules that never bend

- **Abstraction + implementation, never a driver-name branch.** Each per-driver
  behavior is a capability interface with one implementation per engine. Adding
  an engine means writing implementations, not editing a shared `switch`. This
  applies to backend (`internal/engine`) and frontend (`SqlDialect`,
  `DriverHooks`, connection-driver registry) alike.
- **Capabilities are derived from interfaces.** `internal/engine/capability.go`
  reports a capability as available only when the driver implements its
  interface. Do not stub a method to "turn on" a capability; leave the
  interface unimplemented and the capability stays `false` end to end.
- **Secrets never hit logs.** No DSNs, SQL text, bind parameters, auth headers,
  raw query strings, or row values in logs. Log lifecycle events, denied and
  degraded paths, unsupported-capability notices, and worker outcomes only.
- **Tests ship with the code.** Every capability file has a colocated unit test;
  live behavior is covered by a build-tagged integration test.

## 1. Dependencies

- Add the Go driver and any SQL parser/AST library to `go.mod`; run
  `make tidy` and `make audit`.
- Prefer a driver that exposes:
  - a `database/sql`-compatible entry point or a `driver.Connector`,
  - a **custom context dialer** hook (needed for SSH tunnelling — see §11),
  - a programmatic **`*tls.Config`** hook (needed for custom CA / mTLS — see §10).
- Record the parser's license and provenance if it is vendored or adapted
  (see `internal/engine/completioncore/PROVENANCE.md` for the pattern).

## 2. Engine package skeleton

Create `internal/engine/engines/<name>/` and mirror the file layout of
`internal/engine/engines/oracle/`:

| File | Responsibility | Interface(s) |
| --- | --- | --- |
| `<name>.go` | `init()` calls `engine.Register(...)`; `var _ engine.Driver` assertion | — |
| `doc.go` | package doc: libs used, supported version baseline, integration-test instructions | — |
| `driver.go` | connection lifecycle + query/exec paths | `engine.Driver` |
| `ident.go` | identifier quoting / qualification helpers | — |
| `parser.go` | parse SQL → opaque statements + normalized spans | (internal) |
| `classifier.go` | statement kind for RBAC | `classifier.Classifier` |
| `safety.go` | unsafe-statement heuristics (missing WHERE, etc.) | `safety.Checker` |
| `explain.go` | build the EXPLAIN statement sequence | `explain.Explainer` |
| `statement.go` | SELECT/INSERT/UPDATE/DELETE scaffolding | `statement.Generator` |
| `ddl.go` | visual DDL operations + column-type palette | `ddl.Executor` |
| `cursor.go` | forward-only result paging over a live session | `cursor.QueryCursorDriver` |
| `transaction.go` | begin/commit/rollback + savepoints | `transaction.SavepointController` |
| `inspector.go` | schema directory + object detail + scope discovery + lazy definition | `metadata.SchemaInspector`, `metadata.ScopeDiscoverer`, `metadata.DefinitionInspector` |
| `relationships.go` | foreign-key graph per scope | `metadata.RelationshipInspector` |
| `completer.go` | autocomplete + keyword vocabulary + catalog invalidation | `completer.Completer`, `completer.VocabularyProvider`, `completer.CatalogInvalidator` |

Register the package for the binary with a blank import in
`internal/web/drivers.go`. Add the dialect constant to
`internal/engine/driver.go` (`Dialect<Name>`), and an alias in `NormalizeName`
if the engine has common alternate names.

## 3. `engine.Driver` core (`driver.go`)

- `Connect(ctx, engine.ConnectionConfig)`: parse `cfg.DSN` with the driver's
  native parser, apply `cfg.DefaultScope` as the session's default
  schema/database (do it so it survives connection pooling — see the Oracle
  `schemaConnector` wrapper, which runs `ALTER SESSION SET CURRENT_SCHEMA` on
  every pooled connection), then `PingContext` to fail fast.
- Route every statement through one `execer` accessor that returns the open
  `*sql.Tx` when a transaction is active and the `*sql.DB` otherwise, so
  transaction and non-transaction paths share code.
- `QueryWithOptions` returns columns + rows via `cursor.ScanRows(rows, opts)`.
  `ExecuteWithOptions` returns `result.NewExecutionResult(rowsAffected)`.
  Mark the SQL-injection-safe lines with the `codeql[go/sql-injection]`
  comment used elsewhere — editor SQL is user-authored and permission-gated by
  the web layer.
- `Dialect()` returns the registry key.

> **EXPLAIN gotcha:** the EXPLAIN plan output is always a *query* result set.
> The web handler runs `explain.Plan.Setup` for side effects and then the
> `Statement` through the query path. Do not build an EXPLAIN whose plan comes
> back only as `rows_affected` — it renders in the UI as a bare "query
> executed".

## 4. Parsing, classification, safety

- `parser.go`: wrap the AST library. On a syntax error return a
  `*parser.SyntaxError`; callers treat unparseable SQL as "let the database
  reject it", not as a hard failure.
- `classifier.go`: map AST node types to `classifier.Kind`
  (`DQL`/`DML`/`DDL`/`Unknown`). Fold multi-statement scripts conservatively —
  any mix or any `Unknown` yields `Unknown`. Downgrade locking reads
  (`SELECT ... FOR UPDATE`) and anything you cannot positively identify to
  `Unknown`; the RBAC layer treats `Unknown` as maximally privileged.
  Classification always runs on the **original** SQL, never a rewrapped form.
- `safety.go`: at minimum flag `UPDATE`/`DELETE` with no `WHERE`. Return
  `Source: "omni"` (or your parser's name) and per-statement spans.

## 5. EXPLAIN (`explain.go`)

- Implement `ExplainSpec()` (`SupportsAnalyze`) and `Explain(sql, mode)`.
- `Explain` is pure text building. Self-validate: reject multiple statements
  (`explain.ErrMultipleStatements`), an already-EXPLAINed statement
  (`explain.ErrAlreadyExplained`), and unknown modes (`explain.ErrUnsupported`).
- Return an `explain.Plan{Setup, Statement, Teardown}`:
  - `Setup` — side-effect statements run first, results discarded, abort on
    error (e.g. Oracle `EXPLAIN PLAN FOR <stmt>`, or the `ALTER SESSION` pair
    for ANALYZE).
  - `Statement` — runs last; **its result set is the plan shown to the user**.
  - `Teardown` — best-effort cleanup, runs even if `Statement` failed.
- ANALYZE must run only statements the user is already permitted to run (the
  handler applies the per-class permission check before `Setup`). If ANALYZE
  needs a privilege the connection account may lack (Oracle's `V$SQL_PLAN`
  views), document it and let the plan query surface the database's own
  explanatory message rather than erroring.

## 6. DDL, statement generation, transactions

- `ddl.go`: declare an `Operations` list and a **closed `ColumnTypes`
  allowlist** — every interpolated value is either `ident`-quoted or a member
  of that allowlist. Validate with `ddl.Validate` before building SQL.
- `statement.go`: declare a `statement.Spec` of object kinds × operations;
  build with `statement.Build` and the engine's placeholder syntax
  (`:1`/`$1`/`?`).
- `transaction.go`: `BeginTx`/`Commit`/`Rollback` plus `Savepoint` /
  `RollbackToSavepoint` using names from `transaction.NewSavepointName` only.
  If the engine auto-commits on DDL, say so — it drives the frontend
  `manualTransactionWarning`.

## 7. Schema introspection (`inspector.go`, `relationships.go`)

- `SchemaSpec()` is pure and static: declare each object kind with label,
  order, `Relational`, `SupportsDiagram`, and `Listing` (`"enumerated"` vs
  `"searched"` for high-cardinality kinds).
- `InspectDirectory(opts.Root)` — cheap names+kinds listing, scoped to a root.
  Populate `RowCounts` only when it is free alongside the listing query.
- `InspectObjects(refs)` — detail for requested refs only; push the filter
  into the query, never fetch-all-then-filter.
- `DefinitionInspector.InspectDefinition(ref)` — lazy single-object DDL text.
  Implement this instead of embedding a "DDL" source descriptor in bulk
  `InspectObjects` when producing the definition per object is expensive.
- `ScopeDiscoverer.DiscoverScopes` — cheap connection-time hierarchy (list
  schemas/databases only, no object inspection).
- Exclude vendor/system schemas; prefer a runtime "system object" flag with a
  static fallback list.
- **Directory loading strategy:** the default sync is eager (full crawl,
  cached). For cloud data warehouses see `docs/sqlwarden-architecture.md` →
  *Directory Loading Strategy* — a lazy strategy is planned behind a
  `SchemaSpec` capability flag; new warehouse engines should expect to opt into
  it rather than eager-crawling millions of objects.

## 8. Completion (`completer.go`)

- Implement `Complete` against `completioncore` with a
  `completioncore.NewSchemaResolver(index, defaultSchema)` built from the
  immutable `metadata.Index`. **No completer opens a live connection.**
- Quote identifiers on insert unless they are lexically safe/bare for the
  engine's folding rules.
- `CompletionVocabulary()` — keywords (from the parser token table), built-in
  types, and common functions; build once behind `sync.Once`.
- `InvalidateCompletionCatalog(connectionID)` — drop cached indexes on schema
  change.
- Add the dialect's completion package under
  `internal/engine/completioncore/<name>/` if grammar-level candidates are
  needed.

## 9. Frontend wiring

Add `frontend/src/components/ide/engines/<name>/` and register in
`engines/registry.ts` (`frontendEngines` array):

| File | Responsibility |
| --- | --- |
| `engines/<name>.ts` | `FrontendEngine`: `id`, `label`, `brand` (icon + description), `dialect`, `objectDetail`, `diagram`, `connection`, optional `manualTransactionWarning` |
| `engines/<name>/dialect.ts` | `BaseSqlDialect` subclass: `formatObject`, `formatColumn`, `boundedCountQuery`, and a `formatter` from `sqlFormatter.ts` (add the `sql-formatter` language binding there) |
| `connection-drivers/<name>.ts` | `DriverDef`: form `fields` grouped by section, `buildDSN`, `parseDSN` |
| `object-detail/drivers/<name>.ts` | `ObjectDetailHooks`: `headerBadges`, `columnExtras` for engine-specific metadata |
| `assets/drivers/<name>.svg` | brand icon (real vendor logo, not a placeholder) |

The connection-driver and brand registries derive from `frontendEngines`
automatically — no separate registration. Keep identifier quoting in the
dialect consistent with the backend `ident.go` (folding rules must match).

Run `make frontend/format`, `make frontend/lint`, `make frontend/typecheck`,
`cd frontend && bun run test`, and `cd frontend && bun run build`.

## 10. TLS (custom CA / mTLS / verification) — SQLW-114

Connections currently persist one opaque DSN with no structured TLS material.
The planned feature adds:

- A per-engine `TLSSpec` capability declaring supported knobs (verification
  modes, CA-bundle, client-cert pair, SNI override, wallet path).
- Encrypted-at-rest CA PEM / client cert PEM / client key PEM on the
  connection (same secret path as the DSN password; redacted in API responses;
  never logged).
- `Connect` builds a `*tls.Config` from that material and hands it to the
  library: pgx `config.TLSConfig`; mysql `RegisterTLSConfig(uniqueName, cfg)` +
  `?tls=<name>` with `DeregisterTLSConfig` on `Close`; go-ora
  `OracleConnector.WithTLSConfig(cfg)`.

A new engine's driver **should be chosen for a programmatic `*tls.Config`
hook** so it can participate. Until SQLW-114 lands, expose at least an SSL
on/off / verify-mode select in the connection form.

## 11. SSH tunnelling — SQLW-115

SSH is engine-agnostic: it lives in `internal/connection` as a `DialContext`
provider, established before `Connect` and torn down with the session. Each
engine only needs to pass the dialer to its library:

- pgx: `pgconn.Config.DialFunc` (runs before TLS, so a tunnel + DB TLS compose;
  TLS `ServerName` stays the real host).
- mysql: `mysql.RegisterDialContext(name, fn)` + `?net=<name>`.
- go-ora: `SessionInfo.RegisterDial(fn)` / connector `Dialer`.

A new engine's driver **should be chosen for a custom context-dialer hook**.
The stored DSN keeps the real target host:port; the dialer routes it through
the bastion. Host-key verification is mandatory (no default
`ssh.InsecureIgnoreHostKey`).

## 12. Web layer + API

- Blank-import the package in `internal/web/drivers.go`.
- The `/api/v1/engines/<id>` catalog endpoint and its capability payload are
  generic — they derive from the capability set, no per-engine handler code.
- Add endpoint coverage in `internal/web/*_test.go` following
  `handlers_engines_test.go` (engine descriptor, completion vocabulary) and the
  query/explain/DDL handler tests.

## 13. Integration test

- Add `<name>_integration_test.go` with `//go:build integration`.
- Start the database with `testcontainers` (see Oracle's
  `gvenzl/oracle-free` setup) and exercise: connect, query, cursor paging,
  classify, DDL executor, transaction + savepoint, directory + object
  inspection, relationships, EXPLAIN (plain and analyze).
- If the engine advertises cursor support, add the heap-materialization guard
  test (fetching a small page from a large generated result must not grow Go
  heap proportionally) — required per the architecture doc.
- The tag keeps it out of `make test`; document the
  `go test -tags integration ./internal/engine/engines/<name>/...` invocation
  in `doc.go`.

## 14. Definition-of-done checklist

- [ ] `go.mod` updated; `make tidy`, `make audit` clean.
- [ ] All 13 capability files present or a documented reason for each omission.
- [ ] `engine.Register` in `<name>.go`; blank import in `internal/web/drivers.go`.
- [ ] Dialect constant + `NormalizeName` alias in `internal/engine/driver.go`.
- [ ] Colocated unit tests for every capability file; `make test` green.
- [ ] Build-tagged integration test covering every capability; passes locally
      against a container.
- [ ] Frontend engine registered in `engines/registry.ts`; dialect, connection
      driver, object-detail hooks, real brand icon added.
- [ ] `make frontend/format/check`, `make frontend/lint`,
      `make frontend/typecheck`, `bun run test`, `bun run build` all green.
- [ ] Web endpoint coverage added.
- [ ] Connection form exposes at least SSL on/off + verify mode; driver chosen
      to allow a programmatic `*tls.Config` (SQLW-114) and a custom context
      dialer (SQLW-115).
- [ ] `docs/sqlwarden-architecture.md` "Implemented target drivers" list and
      any capability notes updated.
- [ ] Logs reviewed for secret leakage.
