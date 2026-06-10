# SQLWarden — Architecture & Implementation Guide

**Self-Hosted Database Access Platform**
Version 1.2 | June 2026 | Confidential

---

## Table of Contents

1. [Product Overview](#1-product-overview)
2. [Licensing & Open Core Architecture](#2-licensing--open-core-architecture)
3. [System Architecture](#3-system-architecture)
4. [Database Driver Layer](#4-database-driver-layer)
5. [Connector Architecture](#5-connector-architecture)
6. [Transaction & Session Handling](#6-transaction--session-handling)
7. [Access Control & RBAC](#7-access-control--rbac)
8. [Audit Logging](#8-audit-logging)
9. [SQL Parser & Database Edge Cases](#9-sql-parser--database-edge-cases)
10. [Frontend Architecture](#10-frontend-architecture)
11. [Desktop Application — Wails](#11-desktop-application--wails)
12. [PWA — Web Deployment](#12-pwa--web-deployment)
13. [Compliance & Security Posture](#13-compliance--security-posture)
14. [Contributor & Legal Considerations](#14-contributor--legal-considerations)

---

## 1. Product Overview

SQLWarden is a self-hosted database access platform serving three distinct audiences simultaneously from a single deployment. It is architected as an open core product: the SQL IDE, local authentication, workspace/resource model, core RBAC engine, and database access APIs live in the core codebase, while future enterprise features such as SSO/SCIM, tamper-evident audit logging, SIEM forwarding, and advanced compliance packaging may be license-gated.

### The Three-Audience Value Proposition

| Audience | Primary Need | SQLWarden Delivers |
|---|---|---|
| Developers | A capable SQL IDE they will actually use | Schema-aware autocomplete, AI assist, multi-database support |
| DB Ops / Platform | Central access control plane | One front door for all database access, credential management, role scoping |
| Compliance / Security | Immutable audit trail | Every query — who, what, when, from where — recorded and exportable to SIEM |

### Deployment Targets

SQLWarden ships as four distinct deployment targets from a single codebase:

| Target | Description | Auth | Storage |
|---|---|---|---|
| Community Server | Self-hosted, open source | Local username/password | SQLite by default; PostgreSQL supported |
| Enterprise Server | Self-hosted, license-gated features | SSO + local | Server-configured SQLWarden metadata database |
| Desktop (Wails) | Native app, single-user local backend, optional remote backends | Local account/session | SQLite for local backend |
| PWA | Web-deployed instance installed to desktop/mobile | Server-determined | Server-determined |

Database credentials and query history never leave the customer's infrastructure. This is a core product principle and a key differentiator in regulated industries.

> **Target verticals:** Financial services, healthcare (HIPAA), legal, and any SaaS company requiring SOC 2 compliance.

---

## 2. Licensing & Open Core Architecture

### 2.1 License Strategy

SQLWarden uses the Apache 2.0 license for all core functionality. The current repository does not include an `enterprise/` tree. If SQLWarden later adopts a proprietary enterprise package, that package should contain only add-on features such as SSO/SCIM providers, tamper-evident audit logging, SIEM forwarding, and license enforcement; the core RBAC engine stays in core.

| Feature | Community (Apache 2.0) | Enterprise (Proprietary) |
|---|---|---|
| SQL IDE with AI assist | ✓ | ✓ |
| Connector agent | ✓ | ✓ |
| Multi-database support | ✓ | ✓ |
| Core audit log (file/stdout) | ✓ | ✓ |
| Local username/password auth | ✓ | ✓ |
| Desktop app (Wails) | ✓ | ✓ |
| SSO — SAML 2.0, OIDC, LDAP | | ✓ |
| Tamper-evident audit log | | ✓ |
| SIEM forwarding | | ✓ |
| Core RBAC engine | ✓ | ✓ |
| SSO/SCIM-managed identity and provisioning | | ✓ |
| Air-gapped deployment support | | ✓ |
| BAA signing (HIPAA) | | ✓ |
| SOC 2 documentation package | | ✓ |

### 2.2 Current Codebase Structure

The current repository is a Go API plus embedded React SPA. `cmd/api` is intentionally thin; HTTP application code lives under `internal/web` so future entrypoints such as Wails desktop can reuse it.

```
sqlwarden/
├── README.md
├── Makefile
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
├── assets/
│   ├── migrations_postgres/          ← embedded PostgreSQL migrations
│   ├── migrations_sqlite/            ← embedded SQLite migrations
│   └── static/                       ← embedded frontend build output
│
├── cmd/api/
│   └── main.go                       ← server entrypoint
│
├── frontend/                         ← React application (shared across all targets)
│   ├── package.json
│   ├── bun.lock
│   ├── vite.config.ts
│   ├── components.json               ← shadcn/ui component registry
│   └── src/
│       ├── main.tsx
│       ├── routeTree.gen.ts          ← auto-generated route tree (do not edit)
│       ├── styles.css
│       ├── routes/
│       │   ├── __root.tsx            ← provider root
│       │   ├── index.tsx             ← landing / organization selection
│       │   ├── settings.*            ← account and instance settings
│       │   └── orgs.$org_slug.*      ← org, workspace, IDE, access-control routes
│       ├── components/
│       │   ├── app-shell.tsx         ← reusable sidebar shell
│       │   ├── ide/                  ← SQL IDE layout/editor/files/results
│       │   ├── ui/                   ← shadcn/base-ui primitives
│       │   └── theme-provider.tsx
│       └── lib/                      ← API client, permissions, icons, editor themes
│
└── internal/                         ← core Go packages (Apache 2.0)
    ├── access/                       ← RBAC enforcer, permissions, role seeding
    ├── connection/                   ← live DB session manager
    ├── database/                     ← Bun models and query helpers
    ├── driver/                       ← target database driver abstraction
    ├── encrypt/                      ← AES-GCM helpers
    ├── files/                        ← workspace file service
    ├── filestore/                    ← filesystem-backed content store
    ├── password/                     ← bcrypt hashing
    ├── request/                      ← request decoding helpers
    ├── response/                     ← JSON/pagination helpers
    ├── smtp/                         ← SMTP mailer
    ├── token/                        ← JWT and refresh-token helpers
    ├── validator/                    ← input validation
    ├── version/                      ← build version injection
    └── web/                          ← HTTP app, config, routes, middleware, handlers
```

### 2.3 Planned Directory Additions

The following directories do not yet exist but will be added as development progresses:

```
sqlwarden/
├── cmd/
│   ├── api/                          ← existing (community server)
│   ├── api-enterprise/               ← enterprise server entrypoint (build tag: enterprise)
│   └── desktop/                      ← Wails desktop entrypoint
│
├── enterprise/                        ← PROPRIETARY LICENSE
│   ├── LICENSE
│   ├── sso/                          ← SAML, OIDC, LDAP implementations
│   ├── audit/                        ← tamper-evident logger, SIEM forwarder
│   ├── license/                      ← key validation, feature bitmask
│   └── wire.go                       ← registers enterprise impls at startup
│
├── internal/
│   ├── audit/                        ← Logger interface + noop/file impl
│   ├── connhub/                      ← connector WebSocket hub
│   ├── query/                        ← query handler and router
│   └── session/                      ← transaction session store
│
├── connector/                         ← standalone connector agent binary
│
└── pkg/
    ├── result/                        ← normalized ResultSet type
    └── connproto/                     ← shared WebSocket envelope types
```

### 2.4 Key Technology Choices (Current)

| Concern | Implementation | Evidence |
|---|---|---|
| Frontend framework | React + TypeScript | `frontend/src/main.tsx` |
| Frontend router | TanStack Router | `routeTree.gen.ts` present |
| UI components | shadcn/ui | `components.json` present |
| Package manager | Bun | `bun.lock` present |
| Build tool | Vite | `vite.config.ts` present |
| Application database | SQLite | `~/.sqlwarden/sqlwarden.db` by default |
| ORM / query builder | uptrace/bun | App database (accounts, orgs, RBAC, connections, workspace files, auth sessions) |
| Configuration | spf13/viper | Config file, env vars, and CLI flags |
| Email | SMTP | `internal/smtp/` present |
| Password hashing | bcrypt | `internal/password/hash.go` |
| Release management | Release Please | `release-please-config.json` present |

### 2.5 Build Tag Enforcement

Future Go build tags can enforce the open-core boundary at compile time if/when enterprise packages are added. Community binaries must not include enterprise code. The enterprise binary should be produced only through the CI pipeline with the `-tags enterprise` flag and the license public key injected via ldflags.

```bash
# Community server
go build -o dist/sqlwarden ./cmd/api/

# Enterprise server (CI only — requires PUBLIC_KEYS secret)
go build -tags enterprise \
    -ldflags "-X github.com/yourorg/sqlwarden/enterprise/license.rawPublicKeys=$(PUBLIC_KEYS)" \
    -o dist/sqlwarden-enterprise \
    ./cmd/api-enterprise/

# Desktop (via Wails CLI)
wails build
```

The build tag is not the security boundary — it is a compile-time convenience and signal. The **license key is the real enforcement mechanism**. Someone who builds from source with `-tags enterprise` gets a binary with an empty public key (injected only in CI), which means license validation always fails and enterprise features never activate.

### 2.6 License Key System

License enforcement uses Ed25519 asymmetric cryptography. The private key lives only on the SQLWarden license server and never in the repository. The public key is injected into enterprise binaries at build time via ldflags. Validation is entirely offline — no network call required at runtime, essential for air-gapped deployments.

| Property | Detail |
|---|---|
| Algorithm | Ed25519 asymmetric signature |
| Key injection | ldflags at build time — empty string in source |
| Validation | Fully offline — no phone-home required |
| Key rotation | Multiple public keys embedded by version (`v1`, `v2`, ...) |
| License payload | Signed JSON: org ID, features bitmask, expiry, issued-at |
| Revocation | Short expiry (1 year) + revocation list embedded in binary updates |
| Air-gap support | `SQLWARDEN_AIR_GAPPED=true` disables optional telemetry |

The license payload structure:

```json
{
  "kv": "v2",
  "lid": "lic_abc123",
  "oid": "org_xyz",
  "org": "Acme Corp",
  "ft": 15,
  "iat": 1700000000,
  "exp": 1731536000
}
```

### 2.7 Key Rotation Lifecycle

1. **Day 0:** Generate v1 keypair. Store private key in vault. Inject public key into binary via CI secret.
2. **Day 365:** Generate v2 keypair. Embed both `v1` and `v2` public keys in new binary release. Retire v1 private key from active signing use.
3. Existing v1-signed licenses continue to work. All new licenses signed with v2 private key.
4. **Day 730+:** All v1 licenses expired naturally. Drop v1 public key from binary.
5. **Compromise scenario:** Rotate immediately, issue replacement licenses signed with v2, ship binary update dropping the compromised key.

---

## 3. System Architecture

### 3.1 Technology Stack

| Component | Choice |
|---|---|
| Backend language | Go |
| HTTP router | Chi |
| Frontend | React + TypeScript |
| Frontend router | TanStack Router |
| UI component library | shadcn/ui + Base UI primitives |
| Frontend build tool | Vite |
| Package manager | Bun |
| ORM / query builder | uptrace/bun for SQLWarden metadata |
| Configuration | spf13/viper with config file, env, and flags |
| Frontend embedding | Go `embed.FS` |
| Desktop shell | Wails v2 planned |
| Target database drivers | PostgreSQL, MySQL, SQLite currently implemented |
| Application storage | SQLite by default; PostgreSQL support exists; MySQL app DB support remains product direction |
| Workspace file storage | Filesystem-backed store under `~/.sqlwarden/files` by default |

### 3.2 Core Architectural Principles

1. **Single frontend codebase** — `frontend/` is shared across server and future desktop targets.
2. **Reusable web application package** — `cmd/api` is a thin entrypoint; `internal/web` owns config, app wiring, routes, middleware, and handlers so future entrypoints can reuse the same HTTP application.
3. **RBAC by default** — org-owned resources use the same role-binding and resource-hierarchy model in server, local, and future desktop modes. Single-user mode seeds a local org instead of bypassing authorization.
4. **Ownership-separated personal spaces** — `/me` routes use account-owned workspace middleware and are intentionally outside org RBAC.
5. **Uniform target driver surface** — query handlers talk to `internal/driver`; concrete target databases normalize results into `pkg/result`.
6. **Storage abstraction** — app metadata is stored through `internal/database`; workspace file content is stored through `internal/files` and `internal/filestore`.

### 3.3 Request Flow

```
HTTP POST /api/v1/orgs/{slug}/workspaces/{ws_id}/connections/{conn_id}/query
  → authenticateV1
  → requireAccount
  → orgCtx / wsCtx / connCtx
  → query permission classification
  → internal/access enforcer
  → internal/connection session manager
  → internal/driver Query or Execute
  → HTTP 200 with normalized ResultSet
```

### 3.4 Storage Strategy by Deployment Target

| Target | Application Storage | Why |
|---|---|---|
| Community / Enterprise Server | SQLite by default; PostgreSQL supported | Self-hosted, persistent deployment |
| Desktop (Wails) | SQLite at OS user data path | Single-user, zero-setup, no external dependency |
| Local development | SQLite (`~/.sqlwarden/sqlwarden.db`) | Already bootstrapped — zero-config startup |

---

## 4. Database Driver Layer

### 4.1 Unified Driver Interface

All database drivers implement a single Go interface. The handler, router, audit logger, and schema browser never interact with concrete database types.

> **Important distinction:** `internal/driver/` is for *target databases* that users connect to and query. It is entirely separate from `internal/database/`, which uses uptrace/bun against SQLite/Postgres/MySQL to store SQLWarden's own metadata (users, connection configs, audit log, query history).

```go
type Driver interface {
    Connect(ctx context.Context, cfg ConnectionConfig) error
    Ping(ctx context.Context) error
    Close() error

    Query(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)
    Execute(ctx context.Context, sql string, args ...any) (*result.ResultSet, error)

    Tables(ctx context.Context, database, schema string) ([]TableMeta, error)
    Columns(ctx context.Context, database, schema, table string) ([]ColumnMeta, error)

    Dialect() Dialect
}
```

### 4.2 Driver Registry Pattern

Drivers self-register via `init()` functions. Adding a new database requires only a new package and one blank import in `main.go`.

```go
// internal/driver/postgres/postgres.go
func init() {
    driver.Register("postgresql", func(cfg ConnectionConfig) (Driver, error) {
        return newPostgresDriver(cfg)
    })
}

// cmd/api/main.go
import (
    _ "github.com/yourorg/sqlwarden/internal/driver/postgres"
    _ "github.com/yourorg/sqlwarden/internal/driver/mysql"
    _ "github.com/yourorg/sqlwarden/internal/driver/sqlite"
)
```

### 4.3 Normalized Result Type

Every driver translates its native wire format into a single normalized `ResultSet`. This translation happens once, inside the driver — every downstream consumer receives the same shape regardless of database.

```go
type ResultSet struct {
    Columns  []Column
    Rows     []Row      // [][]Value
    RowCount int
    Affected int64
    Duration int64      // milliseconds
    Warnings []string
    Messages []string   // PRINT output, DBMS_OUTPUT, RAISE NOTICE, etc.
}

type Column struct {
    Name     string
    Type     ColumnType  // normalized — e.g. "decimal", "datetime"
    RawType  string      // original — e.g. "NUMERIC(10,2)", "TIMESTAMPTZ"
    Nullable bool
}

// Value is a discriminated union — exactly one field populated
type Value struct {
    Type    ValueType   // null | text | integer | float | decimal | bool | time | bytes
    Text    *string
    Integer *int64
    Float   *float64
    Decimal *Decimal    // string-backed exact precision — never float64
    Bool    *bool
    Time    *time.Time
    Bytes   []byte
}
```

**Decimal values are always string-backed.** `float64` cannot represent `99.99` exactly. Decimal values serialize as JSON strings — never as JSON numbers.

### 4.4 JSON Wire Format to UI

```json
{
  "columns": [
    { "name": "id",         "type": "integer",  "raw_type": "INT4",          "nullable": false },
    { "name": "balance",    "type": "decimal",  "raw_type": "NUMERIC(10,2)", "nullable": false },
    { "name": "created_at", "type": "datetime", "raw_type": "TIMESTAMPTZ",   "nullable": false },
    { "name": "metadata",   "type": "json",     "raw_type": "JSONB",         "nullable": true  }
  ],
  "rows": [
    [1, "9999.99", "2026-01-15T10:30:00Z", "{\"tier\":\"gold\"}"],
    [2, "150.00",  "2026-01-16T08:00:00Z", null]
  ],
  "row_count": 2,
  "affected": 0,
  "duration": 42
}
```

### 4.5 Database-Specific Type Mapping Notes

| Database | Type Gotcha | Normalization Decision |
|---|---|---|
| SQL Server | `BIT` has no native boolean — tiny integer (0/1) | Map to `ColumnTypeBoolean`, normalize to `ValueTypeBool` |
| MySQL | `TINYINT(1)` used as boolean convention | Enable `tinyint1isBool` DSN flag, map to `ValueTypeBool` |
| Oracle | `NUMBER` with no precision — can be int, decimal, or very large | Always map to `ColumnTypeDecimal`, never `float64` |
| Snowflake | `VARIANT` / `OBJECT` / `ARRAY` are semi-structured | Map all three to `ColumnTypeJSON`, return raw JSON string |
| PostgreSQL | `TIMESTAMPTZ` vs `TIMESTAMP` — timezone presence differs | Normalize to UTC; record timezone presence in column metadata |
| SQL Server | `UNIQUEIDENTIFIER` (native UUID) | Map to `ColumnTypeUUID`, return as formatted string |
| PostgreSQL | Arrays (`text[]`, `int[]`, etc.) | Serialize to JSON string representation |
| All | `DECIMAL` / `NUMERIC` — must not use `float64` | String-backed `Decimal` type throughout the entire pipeline |

---

## 5. Connector Architecture

### 5.1 Problem Statement

SQLWarden may not have direct network access to all customer databases — particularly where databases are behind firewalls with no inbound rules. The connector pattern deploys a lightweight agent inside the customer's network that maintains an outbound WebSocket connection to the SQLWarden application.

```
User → App → [WebSocket over TLS] → Connector → Database
                  (connector-initiated outbound)
```

### 5.2 Protocol Design

| Property | Detail |
|---|---|
| Direction | Connector-initiated outbound — no inbound firewall rules needed |
| Message format | JSON envelope: `{ request_id, type, connector_id, payload, timestamp }` |
| Concurrency | Single WebSocket connection per connector; requests multiplexed by `request_id` |
| Write safety | Mutex-protected writes — single WS conn shared across goroutines |
| Response matching | `sync.Map` of `requestID → chan *Envelope`; `Deliver()` routes on arrival |
| Reconnection | Connector reconnects automatically with exponential backoff |
| Heartbeat | Periodic ping/pong to detect stale connections |

### 5.3 State Distribution

| Layer | What It Holds |
|---|---|
| App layer | Connection registry, connector registry (`connectorID → podAddr`), `sessionID → connectorID` map, audit log |
| Connector | Own identity + pre-shared token, DB connection pool, in-flight requests (`requestID → chan Result`), transaction session map (`sessionID → *sql.Conn`) |
| Client / user | Session ID only |

### 5.4 Horizontal Scaling

```
Routing decision tree (gateway):

Has X-Warden-Session header?
  Yes → decode pod prefix from session ID → route to owning pod
  No  → look up connectionID config
          Direct connection?    → round-robin any pod
          Connector connection? → ConnectorRegistry.Locate(connectorID)
                                  → route to pod owning that connector's WS
```

| Concern | Mechanism |
|---|---|
| Session affinity | Session ID encodes pod identity: `"pod-2:abc123"` |
| Connector affinity | Redis/Postgres registry: `connectorID → podAddr` |
| Pod identity | Kubernetes downward API: `POD_NAME` env var |
| Pod addressing | Kubernetes headless service |
| Pod failure | Sessions on dead pod return `410 Gone` |
| Session reaper | Background goroutine expires idle sessions after configurable timeout |

---

## 6. Transaction & Session Handling

### 6.1 The Stickiness Problem

HTTP is stateless but database transactions are stateful. `BEGIN` on connection A is meaningless if `COMMIT` lands on connection B. SQLWarden pins a dedicated `*sql.Conn` for the transaction's lifetime and encodes pod ownership in the session ID so the gateway routes all subsequent requests correctly.

### 6.2 Session Lifecycle

1. Client executes `BEGIN`
2. App pins a `*sql.Conn` from the pool, generates `session_id` with pod prefix (`"pod-2:abc123"`)
3. `session_id` returned to client
4. Client sends subsequent queries with `X-Warden-Session: pod-2:abc123` header
5. Gateway decodes pod prefix, routes to owning pod
6. Pod looks up pinned `*sql.Conn`, uses it for execution
7. `COMMIT` or `ROLLBACK` closes session, returns connection to pool

> **Connector scenario:** The pinned `*sql.Conn` lives inside the connector process. The app hub tracks `sessionID → connectorID` so all transaction queries route to the same connector.

### 6.3 Session Expiry

A background reaper goroutine per pod rolls back and closes sessions idle beyond the configurable timeout (recommended: 30 minutes). This prevents crashed clients from holding database connections indefinitely.

---

## 7. Access Control & RBAC

### 7.1 Implemented RBAC Model

SQLWarden uses a custom RBAC enforcer in `internal/access` for org-owned resources. Authorization is based on:

- organization membership in `org_members`
- account, team, `org_members`, and `workspace_members` principals
- role definitions in `roles` and `role_permissions`
- role bindings in `role_bindings`
- resource ancestry in `resource_hierarchy`

Desktop/single-user mode should seed a local organization and owner policy, then use the same RBAC path as server mode. Personal-space resources under `/api/v1/me` use account ownership middleware instead of org RBAC.

### 7.2 RBAC Data Model

| Concept | Definition |
|---|---|
| Account | Global identity stored in `accounts` |
| Organization | Tenant boundary for roles, teams, bindings, and org-owned resources |
| Team | Org-scoped group principal stored in `teams` / `team_members` |
| Workspace membership | Direct or team-based workspace participation used by the `workspace_members` principal |
| Role | Named set of permission strings with a `scope_type` |
| Role binding | Grants a role to a subject on a resource |
| Evaluation | Additive union semantics; allowed if any matching binding grants the requested permission |
| Default stance | Deny unless an applicable role binding grants permission |

Core org builtin roles are `Owner`, `Administrator`, and `Baseline Access`. Workspace builtin roles are `Workspace Admin` and `Workspace Member`.

Permission namespaces currently include `org:*`, `ws:*`, `wsfile:*`, `env:*`, `conn:*`, and `policy:*`.

### 7.3 Read-Only Connections — Per-Database Support Matrix

| Database | Session-Level Flag | Practical Approach |
|---|---|---|
| PostgreSQL | `SET default_transaction_read_only = on` | Session flag immediately after `Connect()` |
| MySQL | `SET SESSION TRANSACTION READ ONLY` | Session flag — timing sensitive |
| Snowflake | `USE ROLE <read_only_role>` | Role switch at session start |
| SQL Server | None available | Separate read-only credentials required |
| Oracle | Per-transaction only | Grant-based approach; supplement with parser detecting DML in PL/SQL |
| BigQuery | IAM-level only | Separate service account per access tier |

> Separate connection pools per access tier are preferred over toggling session flags on shared pools.

---

## 8. Audit Logging

### 8.1 Audit Interface

```go
type Logger interface {
    Record(ctx context.Context, entry Entry) error
    Query(ctx context.Context, filter AuditFilter) ([]Entry, error)
}
```

Community and desktop ship with `NoopLogger` or `FileLogger`. Enterprise registers a tamper-evident logger with cryptographic checksums per entry.

### 8.2 Audit Entry Fields

Every query produces an entry regardless of success or denial:

`user_id`, `email`, `connection_id`, `database_type`, `full SQL text`, `statement_type`, `target_table`, `rows_returned`, `duration_ms`, `source_ip`, `session_id`, `success`, `error_message`, `deny_reason`, `timestamp`

### 8.3 Special Audit Events

| Event | Handling |
|---|---|
| Denied query | Logged with `allowed: false` and `deny_reason` before HTTP 403 |
| Bulk export | Logged with `rows_returned` count — captures what CLI spool leaves invisible |
| `LOAD DATA` / `COPY FROM` | Detected by SQL parser, flagged as elevated-risk |
| `DBMS_OUTPUT` / `PRINT` messages | Captured in `messages` field alongside the query entry |

---

## 9. SQL Parser & Database Edge Cases

### 9.1 Role of the SQL Parser

1. **Early rejection** — deny disallowed statement types per RBAC before touching the database
2. **Construct interception** — detect CLI-only directives that are not valid SQL
3. **Audit enrichment** — identify target tables and classify statement type

### 9.2 CLI Constructs Requiring Interception

| Construct | Database | Required Handling |
|---|---|---|
| `GO` batch separator | SQL Server | Split script into batches before execution |
| `DELIMITER` directive | MySQL | Client-only — update statement terminator for script execution |
| `/` on its own line | Oracle (SQL*Plus) | Strip before sending — signals end of PL/SQL block |
| `\d`, `\dt`, `\l`, `\copy` | PostgreSQL (psql) | Translate to `information_schema` queries or reject with clear message |
| `DESCRIBE` / `DESC` | MySQL, Oracle | Rewrite to `information_schema.columns` query |
| `SET SERVEROUTPUT ON` | Oracle | Trigger post-execution `DBMS_OUTPUT.GET_LINES` fetch |
| `SPOOL filename` | Oracle | Intercept — trigger streaming download |
| `@filename.sql` | Oracle | Intercept — trigger file upload and sequential execution |

### 9.3 File System Operation Handling

| Operation | Handling Strategy |
|---|---|
| Spool / `\o` export | Stream result as file download (`Content-Disposition: attachment`) |
| File input (`@file`, `\i`, `source`) | Upload via browser File API or native dialog (desktop); execute statement-by-statement with progress |
| `COPY TO/FROM` file path (PostgreSQL) | Detect and rewrite to client-streaming variant |
| `LOAD DATA LOCAL INFILE` (MySQL) | Client-streaming — receive upload, stream to driver |
| `LOAD DATA INFILE` (MySQL, server-side) | Pass through with elevated audit flag and UI warning |
| `BULK INSERT` (SQL Server) | Pass through with audit flag and UI warning |
| `UTL_FILE` (Oracle) | Cannot intercept inside PL/SQL — pass through and audit the block |

### 9.4 Result Set Edge Cases

| Edge Case | Database | Handling |
|---|---|---|
| Multiple result sets | SQL Server, MySQL stored procedures | Present as tabbed results — do not discard after first set |
| `PRINT` / `RAISERROR` messages | SQL Server | Captured out-of-band, displayed as message stream |
| `DBMS_OUTPUT` | Oracle | Auto-fetch `GET_LINES` after execution and display |
| Implicit DDL commits | MySQL, Oracle | Surface warning in UI for mixed DDL/DML scripts |
| `LISTEN` / `NOTIFY` | PostgreSQL | Dedicated WebSocket per browser session to proxy notifications |
| Advisory locks (`GET_LOCK()`) | MySQL | Pin to dedicated connection for session lifetime |

---

## 10. Frontend Architecture

### 10.1 Current State

The `frontend/` directory now contains the application shell, settings pages, organization/workspace management surfaces, and SQL IDE:

- **TanStack Router** — file-based routing with auto-generated `routeTree.gen.ts`.
- **TanStack Query** — API/server-state caching and invalidation.
- **shadcn/ui + Base UI** — component primitives and accessible composition.
- **Vite** — build tooling
- **Bun** — package manager and script runner
- **Theme system** — `theme-provider.tsx`, app-shell preferences, and editor-theme preferences.
- **IDE state** — Zustand, IndexedDB, Y.js, and BroadcastChannel.
- **Code editor** — CodeMirror 6 with lazy-loaded editor themes.

### 10.2 Current Frontend Structure

```
src/
├── main.tsx
├── routeTree.gen.ts            ← auto-generated, do not edit
├── styles.css
├── routes/
│   ├── __root.tsx              ← providers: theme, editor themes, query client, router outlet
│   ├── index.tsx               ← landing / organization chooser
│   ├── login.tsx
│   ├── setup.tsx
│   ├── settings.*              ← account, users, orgs, instance settings
│   └── orgs.$org_slug.*        ← org shell, workspaces, users, teams, roles, policies, IDE
├── components/
│   ├── app-shell.tsx
│   ├── app-sidebar.tsx
│   ├── nav-user.tsx
│   ├── ide/                    ← workspace IDE, panels, editor, tabs, files, results
│   ├── theme-provider.tsx
│   └── ui/                     ← shadcn/base-ui primitives
├── lib/
│   ├── api/                    ← API client, query options, file helpers
│   ├── editor-themes/          ← CodeMirror theme loading/preferences
│   ├── icons/
│   ├── permissions.ts
│   └── utils.ts
```

### 10.3 Frontend Capability Gating

The frontend asks the backend for permission metadata and effective permissions:

- `GET /api/v1/orgs/{slug}/permissions`
- `GET /api/v1/orgs/{slug}/permissions/effective?resource_type=...&resource_id=...`

The backend remains the source of truth for role scope maps, resource applicability maps, permission labels, and descriptions. The frontend keeps stable permission string constants only for simple capability checks.

### 10.4 Auth-Aware Routing

- If setup is incomplete, `/` redirects to `/setup`.
- If no session exists, protected routes redirect to `/login`.
- After login, `/` shows an organization/personal-space chooser unless the user has exactly one organization and no personal space choice is needed.
- Organization routes use the app shell/sidebar; `/orgs/{slug}/ide` uses the IDE layout.
- Desktop/local mode should not be modeled as an auth or authorization bypass. In `ACCESS_MODE=single_user`, first-run setup seeds a local organization and grants the local account owner permissions through normal RBAC.

### 10.5 Build Pipeline

```makefile
frontend-dev:
    cd frontend && bun run dev

frontend-build:
    cd frontend && bun run build

build: frontend-build
    go build -o dist/sqlwarden ./cmd/api/

build-enterprise: frontend-build
    go build -tags enterprise \
        -ldflags "-X .../license.rawPublicKeys=$(PUBLIC_KEYS) -X main.version=$(VERSION)" \
        -o dist/sqlwarden-enterprise \
        ./cmd/api-enterprise/

build-desktop:
    VITE_BUILD_TARGET=desktop wails build
```

---

## 11. Desktop Application — Wails

### 11.1 Architecture Overview

The desktop application is intended to be built with [Wails v2](https://wails.io), which compiles the React frontend and a Go backend into a single native binary for Windows, macOS, and Linux. The desktop app can run a local backend bound to `127.0.0.1` on a random available port. The React frontend talks to this local server exactly as it does in the web deployment.

The local desktop backend serves a single persona: **a developer who wants a low-friction SQL IDE for their own database connections**, without running a separate server or infrastructure. Future desktop builds may also support multiple remote SQLWarden backends for enterprise prod/non-prod separation.

### 11.2 Planned Entrypoint — `cmd/desktop/`

```go
// cmd/desktop/main.go
func main() {
    app := NewApp()
    err := wails.Run(&options.App{
        Title:     "SQLWarden",
        Width:     1280,
        Height:    800,
        Assets:    assets,           // embedded frontend/dist (desktop build)
        OnStartup: app.startup,
        Bind:      []interface{}{app},
    })
}

func (a *App) startup(ctx context.Context) {
    port := startLocalServer(ctx)
    // emit port to React frontend — frontend sets apiBase on receipt
    runtime.EventsEmit(ctx, "server:ready", port)
}
```

The React frontend listens for `server:ready` on mount and sets `apiBase` to `http://127.0.0.1:{port}` before calling bootstrap. A loading screen is shown until the event fires.

### 11.3 Local Server Differences from Server Build

| Concern | Server Build | Desktop Build |
|---|---|---|
| Bind address | `0.0.0.0:PORT` | `127.0.0.1:{random port}` |
| Auth implementation | Local username/password today; SSO later | Real local account/session for local backend; backend-scoped auth for remote backends |
| Access policy | `internal/access` RBAC for org resources; `/me` owner path for personal spaces | Same model; `ACCESS_MODE=single_user` seeds local org and owner policy |
| Application storage | SQLite by default; PostgreSQL supported | SQLite at OS user data path |
| Audit log | `FileLogger` or enterprise tamper-evident | `FileLogger` → `~/.sqlwarden/audit.log` |
| Rate limiting | Yes | No |
| TLS | Required for production | Not used — loopback only |

### 11.4 Local Account + Seeded RBAC

Desktop/local single-user mode should use:

- `DEPLOYMENT_MODE=desktop` for runtime behavior such as app data paths, loopback binding, Wails lifecycle, and backend selection
- `ACCESS_MODE=single_user` for first-run local bootstrap behavior
- a real local account
- a seeded local organization (`local`)
- normal seeded organization owner policy for that account

This avoids a privileged desktop-only code path. The same handlers, middleware, audit hooks, and RBAC checks stay active for org-owned resources.

### 11.5 SQLite Storage for Desktop

The desktop build uses SQLite for all of SQLWarden's own metadata. This is the same storage layer already used in local development. The default local database path is `~/.sqlwarden/sqlwarden.db`; a future desktop shell may map this to an OS-specific application data directory:

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/SQLWarden/sqlwarden.db` |
| Windows | `%APPDATA%\SQLWarden\sqlwarden.db` |
| Linux | `~/.local/share/sqlwarden/sqlwarden.db` |

The `internal/database` package resolves this path at startup based on build target. In the server build, the same package connects to SQLite or PostgreSQL — uptrace/bun handles the ORM layer consistently across supported metadata backends.

### 11.6 Wails IPC Bridge — `desktop/bridge.ts`

`desktop/bridge.ts` is the **only place** in the React codebase that imports Wails-specific APIs. All other components are unaware of the desktop context. In server/PWA builds, `bridge.web.ts` is aliased in its place via Vite's `resolve.alias` and `VITE_BUILD_TARGET`.

```typescript
// frontend/src/desktop/bridge.ts (desktop build only)
export async function openFileDialog(): Promise<string | null> {
    return await window.go.App.OpenFileDialog()
}
export async function saveFileDialog(defaultName: string): Promise<string | null> {
    return await window.go.App.SaveFileDialog(defaultName)
}

// frontend/src/desktop/bridge.web.ts (server / PWA build)
export async function openFileDialog() { return null }
export async function saveFileDialog(_: string) { return null }
```

### 11.7 Desktop Landing Page

When `edition === 'desktop'`, the root route renders a desktop-specific landing page (excluded from server/PWA builds via `VITE_BUILD_TARGET`). It presents:

- **New connection** — inline connection form
- **Recent connections** — loaded from SQLite, shown as quick-connect cards
- **Settings** — local preferences (theme, editor font, keyboard shortcuts)

### 11.8 Build Targets Summary

| Target | Command | Frontend | Auth | Storage |
|---|---|---|---|---|
| Community server | `make build` | Embedded via `embed.FS` | LocalAuth (bcrypt) | SQLite / PG / MySQL |
| Enterprise server | `make build-enterprise` | Embedded via `embed.FS` | LocalAuth + SSO | PG / MySQL |
| Desktop | `wails build` | Bundled by Wails | Local session / backend-scoped auth | SQLite for local backend |
| PWA | n/a (web deploy) | Served by server | Server-determined | Server-determined |

---

## 12. PWA — Web Deployment

### 12.1 Decision

The web-deployed SQLWarden instance is installable as a Progressive Web App. This gives users a standalone window with no browser chrome — functionally equivalent to a desktop app for cloud-connected use cases — at near-zero additional engineering cost.

**The PWA replaces what would otherwise have been a "connect to remote server" mode in the Wails app.** Clean persona separation:

- **Local developer, personal database connections** → Wails desktop app
- **Team / enterprise user accessing company-deployed SQLWarden** → Web browser or installed PWA

### 12.2 Current State

The PWA foundation is **already in place** in the bootstrapped project:

- `frontend/public/manifest.json` — exists, needs completing
- `frontend/public/logo192.png` — exists
- `frontend/public/logo512.png` — exists

The remaining work is adding `vite-plugin-pwa` and completing the manifest content.

### 12.3 What PWA Adds

| Capability | Detail |
|---|---|
| Installability | "Install App" prompt in Chrome, Edge, Safari (iOS 16.4+) |
| Standalone window | No browser chrome — looks and feels like a native app |
| Offline asset caching | Service worker caches JS/CSS/fonts — UI loads instantly |
| App icon on desktop/taskbar | Uses existing `logo192.png` and `logo512.png` |
| Always up to date | Service worker fetches new assets on load — no version coupling |

### 12.4 What PWA Does Not Add

- **Offline query execution** — API calls still require network. Acceptable: you cannot query a database you cannot reach.
- **Native file system access** — file import/export uses the browser File API with permission dialogs. This is the primary UX advantage Wails has over the PWA.
- **System tray / menu bar** — not available in PWA.

### 12.5 Service Worker Cache Strategy

```
JS / CSS / fonts / icons   → Cache-first (hashed filenames from Vite)
/api/*                     → Network-only (never cache API responses)
/index.html                → Network-first with cache fallback
```

The service worker is generated by [vite-plugin-pwa](https://vite-pwa-org.netlify.app/) — configured in `vite.config.ts`. This keeps service worker boilerplate out of the application code.

### 12.6 Manifest Completion

```json
{
  "name": "SQLWarden",
  "short_name": "SQLWarden",
  "description": "Self-hosted database access platform",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#0B1526",
  "theme_color": "#0B1526",
  "icons": [
    { "src": "/logo192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/logo512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

---

## 13. Compliance & Security Posture

### 13.1 Certification Roadmap

| Certification | Priority | Timing | Notes |
|---|---|---|---|
| Pentest + Security Whitepaper | Immediate | Year 1 Q1 | Unblocks ~70% of enterprise procurement. Cost: $5k–$15k. |
| SOC 2 Type I | High | Year 1 Q3–Q4 | Point-in-time. 2–3 months. Unblocks serious mid-market deals. |
| SOC 2 Type II | High | Year 2 | 6-month observation period. ~$15k–$30k + tooling ($10–$20k/yr Vanta/Drata). |
| HIPAA BAA | High | Year 1 | No formal cert — signed BAA template. Legal cost only. |
| ISO 27001 | Medium | Year 2–3 | Required for European enterprise customers. |
| FedRAMP | Low | Only if federal deals materialize | Pursue only with committed federal customer. |

### 13.2 Credential Security Requirements

- Database credentials encrypted at rest using AES-GCM with key from secret store
- Credentials decrypted only when constructing the driver connection — never logged
- TLS 1.2+ enforced for all database connections and connector WebSocket connections
- Connector pre-shared tokens are per-installation and rotatable
- All server API endpoints require authenticated session — no anonymous query execution
- Desktop local server binds to `127.0.0.1` only — not accessible from the network
- `SQLWARDEN_AIR_GAPPED=true` disables all optional telemetry

### 13.3 What Enterprise Procurement Teams Will Ask

Security questionnaires typically probe: software supply chain integrity, vulnerability management, authentication standards (MFA, SSO), data handling and telemetry, encryption standards, and audit log integrity. SQLWarden's self-hosted model significantly reduces the surface area — credentials and query data never transit a third-party system.

---

## 14. Contributor & Legal Considerations

### 14.1 Contributor License Agreement (CLA)

A CLA is required from all contributors before any pull request is merged. Without a CLA, a contributor to the Apache 2.0 core could argue their contribution cannot be included in the proprietary enterprise binary. Use [cla-assistant.io](https://cla-assistant.io) for automated GitHub PR enforcement — one-time signature per contributor.

### 15.2 License Boundary Communication

If/when a proprietary enterprise package is added, the repository README must clearly document the dual-license structure. The `enterprise/` directory must contain:

- Its own `LICENSE` file with the proprietary terms
- A `README.md` explaining which features require a paid license and how to purchase

### 15.3 Build Pipeline Security

| Control | Implementation |
|---|---|
| Public key (license validation) | Injected via CI secrets — never in source |
| Enterprise binary | Built in private CI context, pushed to private container registry |
| Community binary | Built in public CI context, pushed to public Docker Hub |
| Desktop binary | Built and signed via platform CI (macOS notarization, Windows Authenticode) |
| License private key | Stored in HSM or cloud KMS — never on developer machines |
| Release artifacts | Binaries signed; checksums published for user verification |

### 15.4 Release Management

The project uses [Release Please](https://github.com/googleapis/release-please) for automated changelog and versioning, as indicated by `release-please-config.json` in the repo root. Commits following Conventional Commits format (`feat:`, `fix:`, `chore:`) automatically generate `CHANGELOG.md` entries and version bumps on merge to main.

---

*End of Document*

*© 2026 SQLWarden. Internal Use Only.*
