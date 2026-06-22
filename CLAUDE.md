# CLAUDE.md

This file gives AI coding agents working guidance for this repository. For architecture details, read `docs/sqlwarden-architecture.md`.

## Hard Rules

- Use CodeGraph (`codegraph_*`) before grep/read for structural code questions: symbol lookup, call graph, impact, architecture, or flow tracing.
- Use grep/read only for literal text search, docs, or files already identified.
- Never commit unless the user explicitly asks.
- Do not revert user changes unless explicitly requested.
- Keep `frontend/src/routeTree.gen.ts` generated; do not hand-edit it.
- Use Conventional Commits when committing.
- Write code comments only for context that lives in the code itself (non-obvious rationale, invariants, gotchas a future reader needs). Never write comments that narrate the change, the conversation, or the decision history — a maintainer reading the file has none of that context. If a comment only makes sense to someone who saw the original request or PR discussion, omit it or let the code speak for itself.

## Current Architecture Summary

SQLWarden is a Go API plus embedded React SPA.

- `cmd/api` is a thin server entrypoint.
- `internal/web` owns config loading, app wiring, routes, middleware, handlers, and static frontend serving.
- `internal/database` stores SQLWarden metadata through Bun against SQLite/PostgreSQL.
- `internal/access` is the custom RBAC enforcer and permissions catalog.
- `internal/connection` manages live target database sessions.
- `internal/driver` contains target database drivers for PostgreSQL, MySQL, and SQLite.
- `internal/files` and `internal/filestore` implement workspace file metadata/content storage.
- `frontend/` is the React app using TanStack Router, TanStack Query, Tailwind CSS, shadcn/ui, Base UI, CodeMirror, Zustand, IndexedDB, Y.js, and BroadcastChannel.

Future Wails desktop work should reuse `internal/web`; do not put reusable web logic in `cmd/api`.

## Source Of Truth

Use this order when information conflicts:

1. Code, migrations, and tests.
2. `docs/sqlwarden-architecture.md`.
3. `CLAUDE.md`.
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
make frontend/dev
```

Focused commands:

```bash
go test ./internal/web -run TestName -v
go test ./internal/access -run TestName -v
go test ./internal/database -run TestName -v
cd frontend && bun run test
cd frontend && bun run build
```

Migration commands:

```bash
make migrations/new name=migration_name
make migrations/up
make migrations/down
make migrations/goto version=5
```

## Configuration

Configuration uses spf13/viper through `internal/web`.

- Supports config file, environment variables, and CLI flags.
- Default SQLite app DB path is `~/.sqlwarden/sqlwarden.db`.
- Default file storage path is `~/.sqlwarden/files`.
- `deployment_mode` is runtime packaging/context.
- `access_mode` is account/authorization behavior.
- Single-user mode seeds a local org and normal RBAC policies; it is not an authz bypass.

## Backend Conventions

- Keep `cmd/api` thin.
- Put reusable HTTP behavior in `internal/web`.
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
- Centralize API calls/query options in `frontend/src/lib/api`.
- Let API helper/query functions unwrap response envelopes so UI components remain simple.
- Use backend permission catalog data for permission labels/descriptions/scope maps.
- Use Sonner for user-visible mutation/error toasts where needed.
- Avoid adding future-plan/development artifact text into the UI.

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

Frontend:

- Add Vitest coverage for complex state, parsing, and utility behavior.
- Run `cd frontend && bun run test` after frontend logic changes.
- Run `cd frontend && bun run build` when API types, routes, or build-sensitive code changes.

Operational note: database tests may require Docker/testcontainers.

## Go Upgrade Checklist

When changing Go version:

- Update `go.mod`.
- Update GitHub Actions `actions/setup-go` pins.
- Update Docker builder image tag.
- Run `go mod tidy`.
- Run `make audit`.
- Run a local Docker build if release images are affected.

