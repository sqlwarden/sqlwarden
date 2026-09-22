# Backend architecture baseline

Status: Accepted Phase 0 baseline for SQLW-151  
Captured: 2026-09-22  
Reference implementation: `73625231`

This report records the behavior and ownership that later SQLW-151 extractions
must preserve. The Kaneo subtasks are authoritative when this report and the
evolution plan differ.

## HTTP surface and characterization coverage

`internal/web/routes.go` is the public HTTP inventory. It contains 229 explicit
route registrations. Chi expands those registrations to 306 method/path pairs,
including generated method handling. `TestArchitectureRouteInventory` stores a
sorted SHA-256 snapshot of the expanded surface so a handler move cannot
silently drop or remap an endpoint.

The route families are:

- first-run setup and setup status;
- authentication, invitations, account profile, sessions, and engine metadata;
- instance administration, runtime settings, bootstrap diagnostics, and key
  rotation;
- personal and organization workspaces, environments, connections, and
  membership;
- live database sessions, queries, cursors, transactions, history, favorites,
  completion, exports, schema inspection, refresh, and DDL;
- workspace files, revisions, browsing, search, sharing, and content;
- roles, policies, organization membership, and workspace membership;
- durable jobs and job events.

Behavioral characterization already lives beside the handlers in
`internal/web/handlers_*_test.go`, with query/cursor behavior in
`query_cursors_test.go`, schema behavior in `schema_snapshots_test.go`, routing
and middleware behavior in `routes_test.go` and `middleware_*_test.go`, and
startup transport behavior in `server_test.go`. Those tests are the response,
authorization, tenant-boundary, and failure-semantics contract for handlers
that later move behind application services. The route snapshot is the
completeness check across all handlers.

## Process state and long-running work

| Component | Mutable/live state | Starts in | Owner and stop rule |
| --- | --- | --- | --- |
| HTTP listener | sockets and in-flight requests | `allProcessKind.Start` | `allProcessKind.Close` marks it unready and calls `http.Server.Shutdown` first |
| runtime-settings supervisor | current worker settings, update channel, mailer and access-log switches | `startRuntimeSupervisor` | `allProcessKind.Close` cancels `runtimeCancel`, then waits on the application wait group |
| job runners | worker goroutines and claimed durable jobs | runtime-settings supervisor | supervisor stops/replaces runners; process close cancels the supervisor and waits |
| file-content reaper | periodic enqueue timer | `startFileContentDeletionReaper` | `allProcessKind.Close` cancels `fileReaperCancel` and waits |
| target database sessions | session maps, drivers, transactions, pending statements | `connection.Manager.StartReaper` | `internal/app.Application.Close` closes sessions after process kinds and after cursors |
| query cursors | cursor records and driver cursor handles | `QueryCursorManager.StartReaper` | `internal/app.Application.Close` closes cursors before target sessions |
| bounded handler tasks | email notifications, export producers, and schema inspection/write pipeline | request handler through the application wait group or a request context | process-kind drain waits for wait-group tasks; request-scoped pipelines terminate on completion/cancellation |
| application database | Bun/SQL connection pool and durable state | `app.Build` | resource stack closes it last |

The remaining shared mutable transport state is explicit on `web.application`:
the mailer lock, background wait group, file lock map, runtime update channel,
job registry pointer, cancellation functions, and atomic access-log switch.
Schema and completion caches are concurrency-safe service-owned state. Live
target sessions and cursors are process-local state and are never represented
as disposable cache entries.

## Startup, readiness, and shutdown

Construction validates and normalizes bootstrap configuration, creates the
application database directory, opens the application database, runs authorized
migrations, applies startup invariants, then builds authorization, file stores,
encryption, live-session managers, schema/completion services, the durable job
store, and selected process kinds. A failed build closes acquired resources in
reverse order.

Construction starts no goroutines. `Application.Start` starts session and cursor
reapers, then process kinds in configured order. The `all` kind loads validated
runtime settings, applies runtime operations, performs the TLS-data backfill,
binds the HTTP socket synchronously, starts supervisors/reapers, and serves.
Readiness is false before start, after a failed start, as soon as shutdown
begins, when the listener has stopped, or when runtime settings cannot be read.

Shutdown closes process kinds in reverse start order. The `all` kind first
drains HTTP, then cancels background producers and waits for them. The resource
stack subsequently closes query cursors, target sessions, and the application
database in reverse construction order. The application-wide shutdown deadline
bounds process-kind drain and reports an incomplete drain.

## Package dependency and ownership map

At capture time `internal/web` directly imports 40 SQLWarden internal packages.
That broad fan-out is the extraction backlog, not the target design. Ownership
for the first boundary is:

| Package | Owner/responsibility | Allowed outer dependencies |
| --- | --- | --- |
| `internal/config` | bootstrap, secret, and edition configuration loading, validation, precedence, and redacted diagnostics | none of `web`, `rpc`, or `realtime` |
| `internal/app` | dependency construction, resource ownership, process-kind lifecycle, readiness, and reverse shutdown | none of `web`, `rpc`, `realtime`, Enterprise implementations, or Kubernetes APIs |
| `internal/web` | HTTP routing, middleware, transport mapping, and the temporary `all` runtime bridge pending service extraction | may depend on application/domain packages and adapters |
| `internal/connection` | process-local target sessions, transactions, and query cursors | engine contracts and adapters only |
| `internal/database` | durable control-plane persistence and core migrations | database adapter dependencies |
| `internal/jobs` | durable queue store and worker execution | database plus registered job handlers |
| `internal/schema` / `internal/completion` | schema snapshots, caches, inspection orchestration, and completion metadata | engine metadata/capability contracts and cache ports |
| `internal/settings` | validated instance settings and organization/workspace-effective operational policy | never `internal/web`, RPC, or realtime transports |
| `internal/identity` | password identity use cases plus provider-neutral authentication and federation ports | never `internal/web`, RPC, or realtime transports |
| `internal/access` | tenant-safe role and policy administration plus authorization decisions | never `internal/web`, RPC, or realtime transports |
| future `audit`, `catalog`, and `execution` | application use cases and narrow ports | never `internal/web`, RPC, or realtime transports |
| future `ee` | Enterprise composition and decorators | core contracts; core never imports `ee` |

`internal/architecture.TestForbiddenProductionImports` makes the currently
enforceable dependency directions executable. SQLW-163 expands this rule set as
each application package lands. Normal Go compilation remains the package-cycle
gate.

## Supported deployments and current limits

| Shape | Supported now | Limits |
| --- | --- | --- |
| server, single process | yes, `process_kinds=all` | one process owns HTTP, jobs, sessions, and cursors |
| desktop/local composition | yes, the same `app.Build` graph with desktop bootstrap values | SQLite/local filesystem defaults; no distributed dependency |
| application database | SQLite for desktop/single replica; PostgreSQL for server deployments | SQLite is not a distributed control-plane store |
| workspace file storage | filesystem adapter in file or object semantics | S3/object-store adapter is not implemented |
| split `api` / `connector` | selectable; API uses WorkerRuntime over authenticated internal HTTP when connector is separate | one connector replica with a static destination |
| connector replicas | exactly one with the static session directory | more than one fails validation until the Redis directory work lands |
| Redis session directory | named but rejected as unimplemented | SQLW-165 follow-up |
| jobs/realtime/edge-gateway process kinds | named but not implemented | later tickets; Edge Agent and collaboration remain outside SQLW-151 |
| Enterprise modules | configuration category reserved | SQLW-155 introduces the Edition contract and separate composition |

## Performance baseline

The benchmarks are reproducible tests, use local SQLite where persistence is
required, report allocations, and exclude database migration/setup time. They
were run on Linux/amd64 on an AMD Ryzen 9 7900X with Go 1.26.6, three samples of
100 iterations. The table records the median sample; it is a regression signal,
not a service-level objective.

| Path | Benchmark | Median | Allocations |
| --- | --- | ---: | ---: |
| API setup-status through router/middleware/database | `BenchmarkArchitectureAPISetupStatus` | 27.982 us/op | 79-80 allocs/op, 10.7-11.0 KiB/op |
| target SQLite query, 100 materialized rows | `BenchmarkArchitectureQuery` | 57.766 us/op | 530 allocs/op, 34.8 KiB/op |
| durable job enqueue + claim + complete | `BenchmarkArchitectureJobRoundTrip` | 12.139 ms/op | 300 allocs/op, 37.1 KiB/op |
| cached schema directory lookup | `BenchmarkArchitectureSchemaDirectoryCached` | 16.549 us/op | 77 allocs/op, 43.7 KiB/op |

Run all baseline benchmarks with:

```sh
go test -run '^$' -bench '^BenchmarkArchitecture' -benchtime=100x -count=3 \
  ./internal/web ./internal/engine/engines/sqlite ./internal/jobs ./internal/schema
```

On hosts where Testcontainers' Ryuk port allocator conflicts with another test
run, set `TESTCONTAINERS_RYUK_DISABLED=true` for the web package invocation.
This affects package cleanup only; the benchmark itself uses a temporary SQLite
database.

## Phase 0 gate

- The endpoint surface has an executable snapshot and existing behavioral
  characterization coverage.
- Every unbounded background loop has an identified lifecycle owner.
- Package ownership and the first forbidden dependency edges are executable.
- Deployment combinations and limitations are recorded.
- API, query, job, and schema baselines are stored and reproducible.
- Focused suites and static analysis are green; the full suite is run with
  container packages serialized when sharing a Docker daemon.
