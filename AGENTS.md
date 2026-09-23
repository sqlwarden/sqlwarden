# Repository Agent Guidance

This file gives AI coding agents working guidance for this repository. For architecture details, read `docs/sqlwarden-architecture.md`.

`AGENTS.md` is the repository-wide guidance source for coding agents.

## Hard Rules

- Use CodeGraph (`codegraph_*`) before grep/read for structural code questions: symbol lookup, call graph, impact, architecture, or flow tracing.
- Use grep/read only for literal text search, docs, or files already identified.
- Never commit unless the user explicitly asks.
- Do not revert user changes unless explicitly requested.
- Per-driver behavior must use the abstraction+implementation pattern: define a capability interface and implement it per driver, so adding a new database means writing one implementation, not editing a shared `switch`/conditional. Never inline driver-specific logic (SQL shape, quoting, dialect quirks, schema introspection) into shared components or handlers. Examples: backend `internal/engine` engine interfaces (`Driver`, `SchemaInspector`) and per-engine inspectors; frontend `SqlDialect` (`sqlDialect.ts`) and the object-detail `DriverHooks` renderer registry. Provide a sensible default (e.g. a base class) only as an override point, not as a place to branch on driver name.
- Every first-party Go package directory, including test-only packages, must contain a `doc.go` whose package comment accurately describes the package (`// Package name ...`, or `// Command name ...` for `main`). Keep the package comment only in `doc.go`, and update it when the package's responsibility changes. `internal/architecture` enforces this.
- Keep `frontend/src/routeTree.gen.ts` generated; do not hand-edit it.
- Use Conventional Commits when committing.
- Write code comments only for context that lives in the code itself (non-obvious rationale, invariants, gotchas a future reader needs). Never write comments that narrate the change, the conversation, or the decision history — a maintainer reading the file has none of that context. If a comment only makes sense to someone who saw the original request or PR discussion, omit it or let the code speak for itself.

## Current Architecture Summary

SQLWarden is a Go backend plus embedded React SPA, built as one binary that can run as one or more process kinds.

- `cmd/api` is a thin entrypoint: it loads config, builds the Community edition through `internal/community`, and runs the selected process kinds. Keep it thin.
- `internal/config` owns bootstrap configuration (flags, env, secret files, config files, validation, redacted diagnostics).
- `internal/app` is the composition and lifecycle root: it builds the service graph from config, owns every long-running resource, and starts/stops process kinds in order.
- `internal/web` is the HTTP transport (routes, middleware, handlers, static frontend serving) plus the process-kind adapters. It does not load config or construct services, and it reaches target databases only through `internal/execution`.
- Supported process kinds are `all` (API, in-process target execution, and background workers), `api` (public API delegating target execution to a connector), and `connector` (internal execution endpoint). Other kind names in `internal/config` (`jobs`, `realtime`, `edge-gateway`) are reserved and rejected at startup. The only session directory is `static`, so a split deployment runs exactly one connector.
- Application services own use cases and never import transports: `internal/settings` (runtime settings), `internal/identity` (authentication), `internal/access` (RBAC enforcer and permissions catalog), `internal/audit` (durable audit events), `internal/catalog` (orgs, workspaces, environments, connections), plus `internal/schema`, `internal/completion`, `internal/files`, and `internal/jobs`.
- `internal/execution` is the target-execution boundary. `LocalRuntime` runs sessions in-process over `internal/connection`; `WorkerRuntime` forwards them to a connector. Stored target credentials needed at execution time are resolved connector-side through `execution.CredentialProvider` (core implementation `credentials.EncryptedColumnProvider`); an API-only process builds no provider and the internal protocol never carries credentials. Control-plane paths such as connection sealing and key rotation still use the application keyring.
- `internal/engine` contains data-system integrations and capability interfaces. Registered engines: PostgreSQL, CockroachDB, Neon, Supabase, YugabyteDB, MySQL, MariaDB, TiDB, Oracle, SQL Server, and SQLite.
- `internal/edition` defines the edition seam; `internal/community` is the Community Edition root; `ee/` is the Enterprise Edition tree. Core packages never import `ee/`.
- `internal/database` stores SQLWarden metadata through Bun against SQLite/PostgreSQL. `internal/filestore` stores workspace file content.
- `deploy/helm` holds the split-topology Helm chart and its validation tests.
- `frontend/` is the React app using TanStack Router, TanStack Query, Tailwind CSS, shadcn/ui, Base UI, CodeMirror, Zustand, IndexedDB, Y.js, and BroadcastChannel.

New entrypoints (for example a Wails desktop binary) should compose through `internal/app` and reuse `internal/web`; do not put reusable logic in `cmd/api`. `internal/architecture` tests enforce import boundaries, lifecycle conventions, and package documentation.

## Source Of Truth

Use this order when information conflicts:

1. Code, migrations, and tests.
2. `docs/sqlwarden-architecture.md`.
3. `AGENTS.md`.
4. README and older archived docs.

`docs/superpowers/**` may exist locally for reference, but it is not the committed architecture source of truth.

## Common Commands

```bash
make test
make test/cover
make audit
make tidy
make build
make run
make run/live
make frontend/install
make frontend/build
make frontend/format
make frontend/format/check
make frontend/lint
make frontend/typecheck
make frontend/dev
```

Focused commands:

```bash
go test ./internal/web -run TestName -v
go test ./internal/access -run TestName -v
go test ./internal/database -run TestName -v
go test ./internal/architecture -v
cd frontend && bun run test
cd frontend && bun run build
cd frontend && bun run format:check
cd frontend && bun run lint
cd frontend && bun run typecheck
```

Migration commands:

```bash
make migrations/new name=migration_name
make migrations/up
make migrations/down
make migrations/goto version=5
```

## Configuration

Bootstrap configuration is loaded by `internal/config` using spf13/viper.

- Supports config files, environment variables, mounted secret files, and CLI flags.
- `process_kinds` selects which process kinds a binary serves; `all` cannot be combined with other kinds.
- Default SQLite app DB path is `~/.sqlwarden/sqlwarden.db`.
- Default file storage path is `~/.sqlwarden/files`.
- `deployment_mode` is runtime packaging/context.
- `access_mode` is account/authorization behavior.
- Single-user mode seeds a local org and normal RBAC policies; it is not an authz bypass.
- Runtime settings live in the application database and are owned by `internal/settings`, not bootstrap config.

## Backend Conventions

- Keep `cmd/api` thin; wire new services in `internal/app`, not in transports.
- Put use-case rules in application services; `internal/web` handlers decode, call a service, and map results/errors.
- Put reusable HTTP behavior in `internal/web`.
- Target database work goes through `internal/execution`; `internal/web` must not import `internal/connection` or `internal/credentials`.
- Prefer concrete resource permission middleware:
  - `requireOrgPermission`
  - `requireWorkspacePermission`
  - `requireEnvironmentPermission`
  - `requireConnectionPermission`
- Use `internal/request` for JSON decoding and `internal/response` for JSON responses.
- Errors must use the standard envelope:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Name is required.",
    "field_errors": {
      "name": "Name is required."
    }
  }
}
```

- UI-facing paginated lists use `{ "items": [], "page": 1, "page_size": 25, "total": 0 }`.
- Non-paginated list responses should still avoid top-level arrays.
- Keep API JSON lower snake_case.
- Add robust logs for new backend behavior where useful. Use request-aware `logDebug`, `logInfo`, and `logWarn` helpers in `internal/web` for HTTP/domain events so logs carry request/resource correlation. Log high-signal lifecycle events, denied/degraded paths, unsupported capabilities, cache decisions, and background worker outcomes. Do not log request bodies, authorization headers, DSNs, SQL text, bind parameters, raw query strings, or row values.

## RBAC Invariants

- Org membership is the first access gate for org-owned resources.
- RBAC cannot grant org access to accounts outside `org_members`.
- Personal-space routes under `/api/v1/me` are owner-scoped and outside org RBAC.
- Builtin roles are immutable.
- Role bindings are idempotent.
- Role scope validation must happen before custom role creation.
- Permission catalog API is the backend source of truth for permission labels, descriptions, role scope maps, and resource applicability.
- Org owner-level policy grants such as `org:delete` and `org:transfer_ownership` require the actor to already hold the privileged permission.
- Discovery queries must defensively ignore invalid role/resource scope combinations.

## Resource Invariants

- Use helpers such as `InsertWorkspace`, `InsertEnvironment`, and `InsertConnection`; do not raw-insert resources that need hierarchy rows.
- Resource creation must populate `resource_hierarchy`.
- Delete paths must invalidate ancestry cache when relevant.
- Workspace creation must seed workspace builtin roles/policies.
- Environment and connection policy bindings must verify the target resource belongs to the requested workspace.
- Membership removals should revoke affected live DB sessions where implemented.

## Auth And Session Notes

- `POST /api/setup` is self-sealing.
- Multi-user setup creates the first account, instance admin, and first organization.
- Single-user setup seeds a local organization and normal owner policy.
- Auth sessions and org access sessions are database-backed when session revocation is enabled.
- Refresh tokens are stored in DB and rotated.

## Frontend Conventions

- Use shadcn/ui and Base UI primitives.
- Use Tailwind and CSS variables from `frontend/src/styles.css`; avoid ad-hoc color tokens.
- Treat Prettier as the sole formatting authority. Do not hand-format around it or add ESLint formatting rules; run `make frontend/format` after frontend edits.
- Keep ESLint at zero warnings. Do not add blanket rule suppressions; use a narrow suppression with an adjacent rationale only when a framework API makes the rule inapplicable.
- Keep the frontend type-safe under `tsc --noEmit`. Prefer explicit domain types and type-only imports over `any`, unchecked assertions, or duplicated response shapes.
- Centralize API calls/query options in `frontend/src/lib/api`.
- Define shared query keys in `frontend/src/lib/api/query-keys.ts` and keep domain query options in `frontend/src/lib/api/queries`; do not construct competing cache keys in components.
- Let API helper/query functions unwrap response envelopes so UI components remain simple.
- Use backend permission catalog data for permission labels/descriptions/scope maps.
- Use Sonner for user-visible mutation/error toasts where needed.
- Avoid adding future-plan/development artifact text into the UI.
- Keep trivial helpers local when they have one caller. Before adding a helper, search for an existing implementation; move repeated or correctness-critical behavior into a domain-appropriate module with focused tests rather than a catch-all utilities file.
- Keep route and presentation components focused on composition. Extract stateful workflows and reusable domain behavior into named hooks/modules, and colocate focused Vitest tests with them.
- Add database-specific IDE behavior through `frontend/src/components/ide/engines` and its registries. Every supported driver is registered at build time; do not add runtime driver-name conditionals or silent fallback implementations in shared UI code.

## IDE Notes

Current IDE uses:

- CodeMirror 6 for editing.
- Zustand for IDE state.
- IndexedDB for local persistence.
- Y.js and BroadcastChannel for same-browser cross-window sync.
- Backend workspace file APIs for saved files.
- Request cancellation for foreground query cancellation.

Saved files should be treated differently from console scratch state. Browser-local IDE state is acceptable for temporary console/tab layout behavior unless a backend persistence feature is explicitly requested.

## Testing Expectations

Add or update tests with every behavior change.

Backend:

- Handler/middleware/API tests: `internal/web/*_test.go`.
- RBAC tests: `internal/access/*_test.go`.
- DB behavior tests: `internal/database/*_test.go`.
- File service/storage tests: `internal/files/*_test.go`, `internal/filestore/*_test.go`.
- Repository structure, import boundary, and documentation rules: `internal/architecture/*_test.go`.
- Shared behavioral contracts live in `*test` packages (for example `executiontest`, `credentialstest`, `audittest`, `editiontest`, `enginetest`); new implementations of those seams must run the contract.

Frontend:

- Add Vitest coverage for complex state, parsing, and utility behavior.
- Run `make frontend/format/check`, `make frontend/lint`, and `make frontend/typecheck` after frontend changes.
- Run `cd frontend && bun run test` after frontend logic changes.
- Run `cd frontend && bun run build` when API types, routes, dependencies, or build-sensitive code change.

Operational note: database tests may require Docker/testcontainers.

## Go Upgrade Checklist

When changing Go version:

- Update `go.mod`.
- Update GitHub Actions `actions/setup-go` pins.
- Update Docker builder image tag.
- Run `go mod tidy`.
- Run `make audit`.
- Run a local Docker build if release images are affected.
