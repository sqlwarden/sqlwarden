# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Rules

- **Always use codegraph (`codegraph_*` tools) before grep, find, or Read when exploring this codebase.** Codegraph first for any symbol lookup, call graph, or impact analysis; grep/Read only for literal text or when a specific file path is already known.
- **Never commit unless the user explicitly says to.** Complete the work, then wait for the instruction to commit.

## Commands

```bash
# Run all tests (with race detection)
make test

# Run a single Go test
go test ./cmd/api/ -run TestHandlerName -v
go test ./internal/database/ -run TestFunctionName -v

# Run tests with coverage
make test/cover

# Format code and tidy modules
make tidy

# Full audit (fmt, vet, staticcheck, govulncheck, test)
make audit

# Build the binary (also builds frontend)
make build

# Run the application
make run

# Live reload development
make run/live

# Database migrations
make migrations/new name=migration_name   # creates paired up/down files for both postgres and sqlite
make migrations/up                         # apply all pending
make migrations/down                       # rollback one
make migrations/goto version=5             # migrate to specific version

# Frontend development
make frontend/dev      # dev server on :3000 with API proxy
make frontend/build    # production build
make frontend/install  # install bun dependencies
```

## Architecture

SQLWarden is a database access control platform. The Go backend serves a compiled React SPA and exposes a REST API at `/api/v1`.

### Application Struct Pattern

All HTTP handlers are methods on `*application` in `cmd/api/`. The struct is initialized in `main.go` with all dependencies injected:
- `db` — Bun ORM database handle
- `mailer` — SMTP email sender
- `encrypter` — AES-GCM encryption for connection credentials
- `enforcer` — Casbin RBAC access enforcer
- Configuration fields (keys, base URL, etc.)

### Request Lifecycle

1. Chi router matches route → middleware chain executes
2. Middleware resolves context values: authenticated account → organization (from URL slug) → workspace (from URL ID)
3. `requireOrgRole` or `requirePermission` enforces authorization
4. Handler reads request with `internal/request`, writes response with `internal/response`
5. Errors route through centralized helpers in `errors.go` (serverError, notFound, badRequest, failedValidation, notPermitted)

### Access Control

Custom RBAC enforcer in `internal/access/` (replaced Casbin). Two binding types stored in the DB:
- **Role bindings** (`role_bindings`): assign a role to a subject (account or team) at a resource
- **Permission bindings** (`permission_bindings`): grant a single permission directly to a subject at a resource

Permission checks traverse the resource ancestry chain (connection → workspace → org) so org-level bindings cover all descendant resources. See `docs/superpowers/architecture/rbac-and-authorization.md` for the full model.

### Database Layer

`internal/database/` contains one file per entity, each exporting a set of functions (not a repository struct). Functions accept a `*bun.DB` and return domain types.

Supported drivers: PostgreSQL (pgx), SQLite (modernc), MySQL — selected via `DB_DRIVER` env var. Migration files are maintained separately for postgres (`assets/migrations_postgres/`) and sqlite (`assets/migrations_sqlite/`).

IDs use ULIDs (`github.com/oklog/ulid/v2`) — sortable, globally unique.

### Testing

Tests use real databases via `testcontainers-go` (no mocks for the database layer). The test suite uses a singleton container per database type and bounded connection pools to avoid exhaustion.

- Handler tests live in `cmd/api/*_test.go`, using a test application instance
- Database tests live in `internal/database/*_test.go`, using testcontainers
- `testmain_test.go` and `testutils_test.go` in `cmd/api/` set up shared test infrastructure

When running database tests locally, Docker must be available for testcontainers.

### Multi-Tenant Model

`orgs` → `workspaces` → `environments` / `connections`. Accounts belong to an org, have org-level roles, and can be granted fine-grained permissions at any resource level. URL structure: `/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}`.

### Cookies and Auth

- JWT tokens for API auth (`internal/token/`)
- Signed cookies via HMAC-256 and encrypted cookies via AES-GCM (`internal/cookies/`)
- Refresh tokens stored in DB, used to issue new JWT access tokens
- **Auth and SSO are in initial phases** — interfaces and flows are subject to significant change

### Frontend

React 19 + TypeScript SPA in `frontend/`. Built with Vite, routed with TanStack Router (file-based routes in `src/routes/`), styled with Tailwind CSS 4 + shadcn/ui components. Built assets are embedded into the Go binary and served as a SPA fallback.

## Conventions

- **Commit messages**: Conventional Commits required (`feat:`, `fix:`, `chore:`, etc.) — enforced in CI. Use lean single-line messages only — no body, no `Co-Authored-By` trailer
- **Version injection**: `internal/version` package receives `Version`, `Revision`, `BuildDate` via ldflags at build time
- **Migrations**: Always create paired up/down files; separate files for postgres vs sqlite when SQL differs
- **Environment config**: All config via env vars, parsed in `main.go` using `internal/env` helpers with defaults
- **Terminology**: Use "account" everywhere — never "user" — in routes, handler names, JSON fields, and variables

## Invariants

These are load-bearing constraints. Violating them silently breaks authorization or data integrity.

### Setup and registration
- `POST /api/setup` must be called before `POST /api/v1/auth/register` is usable. `registerAccount` checks `HasAnyInstanceAdmin` and returns 403 if setup is not done.
- `POST /api/setup` is self-sealing: it returns 409 after the first successful call. Never call it conditionally in application startup code.

### Org membership is the access gate
- `orgCtx` middleware verifies `org_members` before setting org on context. An account that is not in `org_members` cannot reach any org resource, regardless of any policy bindings.
- When creating an org, always call `db.AddOrgMember` AND `enforcer.SeedOrg` in the same handler. If either is skipped the org will be inaccessible or have no roles.

### Resource hierarchy must be populated at creation time
- `InsertWorkspace`, `InsertEnvironment`, and `InsertConnection` write `resource_hierarchy` rows automatically. Do not bypass these methods with raw inserts or the ancestry chain will be broken and permission inheritance will silently fail.
- `DeleteWorkspace`, `DeleteEnvironment`, `DeleteConnection` cascade-delete hierarchy rows via FK. Always call `enforcer.InvalidateAncestry(resourceType, resourceID)` after deleting a resource so the cache does not serve stale ancestry.

### Workspace seeding
- When creating a workspace, always call `enforcer.SeedWorkspace(ctx, orgID, wsID, creatorAccountID)` immediately after `InsertWorkspace`. This creates the `ws:admin` and `ws:member` builtin roles and binds the creator to `ws:admin`. Without it the workspace has no roles and nobody can administer it.

### Builtin roles are immutable
- `enforcer.DeleteRole` rejects roles with `is_builtin=true`. Never set `is_builtin=true` on custom roles. Never attempt to delete builtins.

### Policy binding idempotency
- `GrantPermissions` uses `INSERT ON CONFLICT DO NOTHING`. Granting the same permission twice is safe and returns no error. Do not add a pre-check before granting.
- `BindRole` uses the same pattern. Role bindings are also idempotent.

### Cross-resource forgery prevention
- Before creating a policy binding for an environment or connection resource, validate it belongs to the target workspace. `grantWorkspacePolicy` does this inline. Any future endpoint that creates bindings must do the same validation.

### Permission scope rules
- `org:*` permissions are only valid at org scope. They cannot be granted on a workspace or connection resource — `ValidForScope` enforces this in `CreateRole` and must be called before any custom role creation.
- `policy:modify` at org scope covers all workspaces in that org (inheritance). Do not grant it at workspace scope unless you intend workspace-only policy control.

