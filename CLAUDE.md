# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

Two-layer RBAC system:
- **Org roles**: `owner > admin > member` — checked via `requireOrgRole` middleware
- **Fine-grained permissions**: Casbin enforcer with action hierarchy `connect < query < execute < manage` on resources like `members`, `teams`, `connections`

Casbin adapter reads/writes policy from the database (`internal/access/adapter.go`). The model is at `internal/access/model.conf`.

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

`tenants` → `workspaces` → `connections`. Users belong to a tenant (org), have org-level roles, and can be granted fine-grained permissions per workspace connection. URL structure reflects this: `/api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections/{conn_id}`.

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
