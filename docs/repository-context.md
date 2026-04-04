# Repository Context

Updated: 2026-04-04

## Source-of-truth order

When project docs disagree, use this order:

1. Code, migrations, and tests
2. `CLAUDE.md`
3. `docs/superpowers/architecture/rbac-and-authorization.md`
4. `docs/superpowers/specs/*.md` and `docs/superpowers/plans/*.md`
5. `docs/sqlwarden-architecture.md`
6. `README.md`

Notes:
- `README.md` is not current enough to drive implementation decisions.
- `docs/sqlwarden-architecture.md` mixes current architecture with planned/open-core future structure and should be read as product direction, not exact repository state.

## Current product shape

SQLWarden is currently a Go API plus embedded React SPA for database access management and query execution.

What is implemented now:
- One backend binary in `cmd/api`
- REST API under `/api/v1`
- JWT auth with refresh token rotation
- First-run bootstrap via `POST /api/setup`
- Instance-admin layer above orgs
- Org/workspace/environment/connection resource model
- Custom RBAC enforcer in `internal/access`
- Personal spaces under `/api/v1/me`
- Live database sessions through `internal/connection`
- Driver abstraction with PostgreSQL, MySQL, and SQLite target database drivers
- Bun-based app database layer with PostgreSQL and SQLite support
- Embedded frontend build served by the Go binary

What is not implemented yet or is explicitly future work:
- Distributed RBAC cache invalidation
- Binding expiry enforcement
- Effective-permissions introspection
- Audit log for policy changes
- Service accounts/API keys
- Deny rules
- Enterprise/community code split described in the architecture guide
- Desktop/Wails target
- SSO and broader enterprise features

## Architecture map

### Backend entrypoint

`cmd/api/main.go`
- Loads env-based config
- Opens app database
- Runs migrations automatically when `DB_AUTOMIGRATE=true`
- Initializes SMTP mailer, AES key derivation, RBAC enforcer, and connection manager
- Serves HTTP routes from `cmd/api/routes.go`

### Request model

Main flow:
- `authenticateV1` parses bearer JWT and loads account
- `requireAccount` enforces auth where needed
- `orgCtx` enforces org membership before any org resource access
- Resource context middleware resolves workspace/environment/connection ownership
- Permission checks are handled by `requirePermission(...)`

Important access rule:
- Org membership is the first access gate
- RBAC never grants access to an org resource unless the account is in `org_members`

### App database

`internal/database`
- One file per domain area, no repository struct pattern
- Bun ORM over PostgreSQL or SQLite
- Embedded migrations under `assets/migrations_postgres` and `assets/migrations_sqlite`

Key tables in current schema:
- `accounts`
- `instance_admins`
- `organizations`
- `org_members`
- `teams`, `team_members`
- `workspaces`
- `environments`
- `connections`
- `roles`, `role_permissions`
- `role_bindings`, `permission_bindings`
- `resource_hierarchy`
- `refresh_tokens`

ID strategy currently implemented:
- Stable relational entities use `int64` / database-generated integer IDs
- Refresh tokens remain ULID text
- Session IDs in `internal/connection` are ULIDs

### Authorization

`internal/access`
- Custom enforcer replaces Casbin
- In-memory cache for org policy, ancestry, and team principals
- Permission checks evaluate:
  - account principal
  - team principals in that org
  - current resource plus ancestor resources from `resource_hierarchy`

Builtins:
- Org builtin roles: `owner`, `admin`
- Workspace builtin roles: `ws:admin`, `ws:member`

Important design constraints:
- Builtin roles are immutable
- Resource hierarchy rows must be written on create paths
- Ancestry cache must be invalidated on delete paths
- Direct bindings and role bindings are idempotent

### Personal spaces

`/api/v1/me`
- Already implemented, not just planned
- No org context or permission middleware
- Ownership enforced by `spaceWsCtx`, `spaceEnvCtx`, `spaceConnCtx`
- `owner_type="space"` causes `enforcer.Can(...)` to short-circuit to allow

Schema support:
- Personal workspaces have `org_id = NULL`, `owner_type = 'space'`
- Migration `000011_space_workspace_unique` enforces unique workspace names per owner

### Query execution

`internal/driver`
- Target database driver interface
- Implementations for PostgreSQL, MySQL, SQLite

`internal/connection`
- In-memory session manager keyed by `(accountID, connectionID)`
- Session reuse and idle reaping
- Query/execute calls serialized per live session

`pkg/result`
- Normalized result-set abstraction returned by drivers and query endpoints

## Route surface

High-level implemented groups in `cmd/api/routes.go`:
- `POST /api/setup`
- `/api/v1/auth/*`
- `/api/v1/instance/admins`
- `/api/v1/account`
- `/api/v1/account/orgs`
- `/api/v1/me/...`
- `/api/v1/orgs/{org_slug}/...`

Within org routes:
- org members
- teams and team members
- org-level roles
- workspaces
- workspace-level roles
- workspace policies
- environments
- connections
- connect/query endpoints

Current list-contract rule:
- UI-facing org-scoped list endpoints use a shared paginated envelope: `items`, `page`, `page_size`, `total`
- Shared list query params are `page`, `page_size`, `sort`, `order`, and `q`
- Resource-specific filters stay flat query params such as `role`, `slug`, or `name`
- New UI-facing list resources should follow the same contract instead of introducing endpoint-specific shapes
- The shared paginated envelope currently lives in `internal/response`; that is cleaner than keeping HTTP response shapes in `internal/database`
- Current DB list methods may still return `response.Paginated[T]`; if stricter layering is needed later, move to `items + total` from the DB layer and assemble the envelope closer to the handler/service layer

## Roadmap state inferred from docs plus code

Completed recently:
- Casbin removal and custom RBAC rollout
- Integer ID migration
- Environments resource
- Workspace-scoped roles and consolidated workspace policy APIs
- Personal spaces under `/me`

Near-term backlog from docs:
- RBAC correctness fixes:
  - enforce `expires_at`
  - address stale cache behavior in multi-instance deployments
- Visibility/debugging:
  - effective permissions API
  - better authorization introspection
- Behavior gaps:
  - enforce `connection.access_mode`
  - add policy audit trail

Longer-range product direction:
- enterprise/open-core split
- SSO
- desktop target
- broader audit/compliance surface

## Known mismatches and cautions

- `docs/sqlwarden-architecture.md` still describes future directories like `enterprise/`, `cmd/desktop/`, and additional packages that do not exist in this tree.
- The RBAC future assessment correctly calls out that `ExpiresAt` exists in schema/models but is not enforced in the enforcer cache/evaluation path.
- The same assessment also correctly notes cache invalidation is process-local only.
- `README.md` should not be used as implementation truth.
- `.codex` in repo root is currently an empty file, not a memory directory.

## Testing reality

Test coverage is substantial in the backend:
- `cmd/api/*_test.go` covers handlers, middleware, setup flow, auth, RBAC behavior, personal spaces, and integration flows
- `internal/access/*_test.go` covers inheritance, team bindings, cache invalidation, workspace-scoped roles, and policy behavior
- `internal/database/*_test.go` covers CRUD plus accessible-resource filtering queries

Operational note:
- DB-layer tests use testcontainers, so local Docker availability matters.

## Commands worth remembering

Core commands from `Makefile` and `CLAUDE.md`:
- `make test`
- `make test/cover`
- `make audit`
- `make tidy`
- `make build`
- `make run`
- `make run/live`
- `make frontend/install`
- `make frontend/build`
- `make frontend/dev`

## Change guidance for future work

When touching auth or RBAC:
- Check routes, middleware, handlers, enforcer, migrations, and tests together
- Prefer permission constants from `internal/access/permissions.go`
- Preserve org membership checks in addition to permission checks

When creating resources:
- Use DB helpers like `InsertWorkspace`, `InsertEnvironment`, and `InsertConnection`
- Do not bypass hierarchy-writing logic with raw inserts

When deleting resources:
- Remove DB record through the helper
- Invalidate ancestry cache where needed

When adding new permissioned resource types:
- Extend permission constants and scope map
- Define hierarchy population
- update enforcer invalidation paths
- add handler and DB coverage

When adding new list endpoints for UI/API clients:
- Return the shared paginated envelope even when current result sets are small
- Support the shared list query params and add only concrete resource-specific filters
- Add handler tests for defaults, pagination, search, filters, sort, and validation
- Add DB tests covering the same filtering and ordering semantics
