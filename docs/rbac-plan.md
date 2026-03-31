# SQL IDE — Auth, RBAC & Multi-Tenancy Design Document

> This document captures all design decisions, rationale, deferred items, and implementation guidance for authentication, authorization, role-based access control, multi-tenancy, session management, JIT access, approval flows, licensing, and extensibility planning for the SQL IDE product.

---

## Table of Contents

1. [Product Editions](#1-product-editions)
2. [Licensing](#2-licensing)
3. [Spaces vs Organizations](#3-spaces-vs-organizations)
4. [Resource Hierarchy](#4-resource-hierarchy)
5. [Numeric IDs](#5-numeric-ids)
6. [Permission Taxonomy](#6-permission-taxonomy)
7. [Role Model](#7-role-model)
8. [Mixed-Scope Roles — No Implication Map](#8-mixed-scope-roles--no-implication-map)
9. [Multiple Owners](#9-multiple-owners)
10. [Permission Resolution and Enforcement](#10-permission-resolution-and-enforcement)
11. [Performance — Batch Permission Resolution](#11-performance--batch-permission-resolution)
12. [SQLite Support](#12-sqlite-support)
13. [Community Edition Defaults](#13-community-edition-defaults)
14. [Enterprise Edition Features](#14-enterprise-edition-features)
15. [Community to Enterprise Upgrade Path](#15-community-to-enterprise-upgrade-path)
16. [Session Management](#16-session-management)
17. [Multi-Org Login and SSO Re-verification](#17-multi-org-login-and-sso-re-verification)
18. [Ownership Deprovision Cascade](#18-ownership-deprovision-cascade)
19. [JIT Query Access](#19-jit-query-access)
20. [Approval-Based Connection Access](#20-approval-based-connection-access)
21. [Designated Approvers](#21-designated-approvers)
22. [Author Bypass and Resource Ownership](#22-author-bypass-and-resource-ownership)
23. [Private vs Shared Visibility](#23-private-vs-shared-visibility)
24. [Connection Secrets Separation](#24-connection-secrets-separation)
25. [Audit Logging](#25-audit-logging)
26. [Query Execution Logging](#26-query-execution-logging)
27. [Data Isolation and Cross-Org Security](#27-data-isolation-and-cross-org-security)
28. [Extensibility — Future Resource Types](#28-extensibility--future-resource-types)
29. [Error Messages and Permission Denial UX](#29-error-messages-and-permission-denial-ux)
30. [Pre-Implementation Checklist](#30-pre-implementation-checklist)
31. [Deferred TODO Items](#31-deferred-todo-items)

---

## 1. Product Editions

The SQL IDE ships in three editions built from the same codebase using build tags and feature gating. The database schema is identical across all editions. The edition determines which features are active, not what data structures exist.

### Desktop Edition

- Single user, runs locally
- SQLite database
- No authentication (the user is the only user)
- No RBAC (god-mode enforcer, always returns true)
- No teams, no orgs, no SCIM, no SSO
- The user operates in their personal Space
- Purpose: individual productivity, adoption funnel

### Community Edition

- Multi-user, self-hosted
- PostgreSQL (recommended) or SQLite (small teams)
- Instance-level authentication (email/password or instance SSO)
- Three predefined org-level roles: Owner, Admin, Member
- No per-resource permissions (all access is org-level)
- No custom roles
- No policy management UI
- SCIM and SAML routes exist but return stubs
- Audit log routes exist but return empty
- Purpose: small team collaboration, bottom-up enterprise entry

### Enterprise Edition

- Multi-user, self-hosted or managed
- PostgreSQL required
- Full SSO (OIDC + SAML) per org
- SCIM provisioning and deprovisioning
- Per-resource permission bindings (environment-level, connection-level)
- Custom roles
- Full policy management UI
- Full audit log
- JIT access requests and approval flows
- Designated approvers
- Purpose: revenue, large team governance and compliance

### How Edition Gating Works

The codebase uses a directory structure where enterprise-only code lives under an `enterprise/` directory. Community builds exclude this directory via build tags. The routes, handlers, and service implementations for enterprise features are replaced with stubs in community that return appropriate responses (e.g., "upgrade to enterprise" or empty data).

The enforcer, the database schema, and the core resource management are shared across all editions. The difference is which routes are live and which features are active.

---

## 2. Licensing

### Core (everything outside `enterprise/`): BSL 1.1

Business Source License 1.1 provides source-available access with one restriction: the software cannot be offered as a competing managed service. After a set period (4 years from each release), each version automatically converts to Apache License 2.0.

**Why BSL over alternatives:**

- **Apache 2.0** is too permissive. Any cloud provider can fork and host it as a competing service without contributing back.
- **AGPL v3** scares enterprises. Many companies have blanket policies against AGPL. The sales team spends time explaining the license instead of selling the product.
- **SSPL** is not OSI-approved. Debian, Fedora, and Red Hat won't package it. Most enterprises treat it as proprietary.
- **ELv2** is viable but lacks the time-based conversion to open source that BSL provides, which is a stronger community signal.

**BSL specifics:**

- Additional Use Grant: production use is permitted except offering the software to third parties as a managed service that allows them to create SQL workspaces and execute queries against their own database connections.
- Change Date: 4 years from release date.
- Change License: Apache License, Version 2.0.

### Enterprise directory: Proprietary

The `enterprise/` directory requires a paid commercial license. This is non-negotiable. Enterprise features are the revenue.

### Contributor License Agreement (CLA)

Required for all contributors. The CLA grants the project the right to license contributions under both BSL and the proprietary enterprise license. Without this, a contributor could argue their code cannot be included in the enterprise edition. A CLA bot (such as CLA Assistant) enforces signing before PR merge.

---

## 3. Spaces vs Organizations

The SQL IDE has two ownership contexts for resources: personal Spaces and team Organizations.

### Space

A Space is a user-scoped personal container. It is **virtual** — no record exists in any database table. The user ID is the Space ID. The concept of "each user has a personal workspace area" lives entirely in the application layer.

**Why virtual:**

- Creating N database records for N users pollutes every table and index with single-user data that nobody else ever accesses.
- Every team feature (SCIM, SSO, teams, roles, policies, audit, approval flows) needs a special case to skip for personal spaces.
- SCIM provisioning and deprovisioning have no relationship to personal spaces.
- Ownership cascading breaks — every deprovisioned user would trigger a cascade for their personal space with nobody to transfer to.
- Sharding by org_id becomes polluted with a 4000:1 ratio of meaningless personal records to meaningful team records.

**How it works:**

- The user's `id` in the `users` table serves as the Space identifier.
- Resources belonging to a Space use `owner_type = 'space'` and `owner_id = <user_id>` in their tables.
- The enforcer short-circuits for Spaces: the user owns everything in their Space, always permitted, no RBAC resolution needed.
- The URL structure uses `/spaces/@me/...` where `@me` is resolved to the authenticated user's ID by middleware.
- Spaces have no teams, members, roles, SCIM, SSO, policies, or audit logs.
- Spaces are always accessible after login. No SSO re-verification needed.

### Organization

An Organization is a team-scoped container backed by a real record in the `organizations` table with full settings, billing, SSO configuration, team management, and RBAC.

**How it works:**

- Real record in `organizations` table with all team settings.
- Resources belonging to an Organization use `owner_type = 'org'` and `owner_id = <org_id>`.
- Full RBAC resolution via the enforcer.
- URL structure uses `/orgs/:orgID/...`.
- SSO, SCIM, teams, roles, policies, audit, approval flows — all active.

### Resource Ownership Columns

Every resource table carries two columns for ownership:

| Column | Type | Description |
|---|---|---|
| `owner_type` | `TEXT NOT NULL` | `'space'` or `'org'` |
| `owner_id` | `BIGINT NOT NULL` | User ID if space, Org ID if org |

These columns are denormalized from the parent workspace onto every child resource table. This enables partition/shard queries without joins and straightforward RLS policies.

### The Context Switcher

The UI presents a context switcher:

- "My Space" is always the first entry and always available.
- Team organizations appear below, listed from the user's session memberships.
- Switching from Space to an Org may trigger SSO re-verification (see Section 17).

### Migration from Space to Organization

When a user outgrows their personal Space and wants to collaborate, they create an Organization and can transfer workspaces from their Space into it. This is a bulk update of `owner_type` and `owner_id` on the workspace and all its child resources (environments, connections, queries, files). Once transferred, the workspace is governed by org RBAC.

---

## 4. Resource Hierarchy

Resources form a tree rooted at the ownership context (Space or Organization):

```
Space or Organization
└── Workspace (≈ GitHub repository)
    ├── Environment (≈ branch, RBAC-protected)
    │   └── Connection (actual DB connection, bound to an environment)
    ├── Connection (workspace-level, shared across environments)
    ├── Query (saved SQL)
    ├── Job (scheduled query execution)
    │   └── JobRun (execution history)
    ├── File (SQL files, notebooks, docs)
    └── Dashboard (future)
        └── Panel (future)
```

### Hierarchy Rules

- A connection can belong to an environment (environment-scoped) or directly to a workspace (workspace-scoped, shared across environments). This is modeled by the connection having an optional `environment_id` foreign key.
- Environments always belong to a workspace.
- Queries, jobs, files, and dashboards belong to a workspace directly. They are not children of environments. If a query targets a specific environment, it stores a `target_environment_id` field, but its RBAC parent is the workspace.
- Cross-workspace resource references are not allowed. Workspaces are fully isolated containers.

### Hierarchy Storage

A `resource_hierarchy` table records parent-child relationships:

| Column | Type |
|---|---|
| `child_type` | TEXT NOT NULL |
| `child_id` | BIGINT NOT NULL |
| `parent_type` | TEXT NOT NULL |
| `parent_id` | BIGINT NOT NULL |
| `owner_type` | TEXT NOT NULL |
| `owner_id` | BIGINT NOT NULL |

This table is maintained by application code whenever resources are created, moved, or deleted. It enables the enforcer to walk from any resource up to the root to collect permissions from ancestor bindings.

---

## 5. Numeric IDs

All internal primary keys and foreign keys use `BIGINT` (or `BIGSERIAL` for auto-increment). UUIDs are reserved only for externally exposed, non-guessable identifiers.

**Why:**

- B-tree indexes on `BIGINT` are faster for joins, sorting, and range scans than UUIDs (128-bit, random distribution causes page splits).
- Smaller storage footprint (8 bytes vs 16 bytes).
- Sequential insertion is cache-friendly.

**Where UUIDs are still used:**

- API tokens (the token string itself, not the row ID)
- Session IDs
- Invite tokens
- Any identifier exposed to external systems where guessability is a security concern

**Table structure pattern:**

```
id          BIGSERIAL PRIMARY KEY     -- internal, fast joins
token       UUID                       -- external, non-guessable (only where needed)
```

---

## 6. Permission Taxonomy

Permissions follow the format `<resource_type>:<action>`. They are strings, not enums, allowing new permissions to be added without schema changes.

### Organization Permissions

| Permission | Description |
|---|---|
| `org:read` | View org settings |
| `org:write` | Edit org settings |
| `org:delete` | Delete the organization |
| `org:invite` | Invite or remove members |
| `org:assign_roles` | Assign and change member roles |
| `org:transfer_ownership` | Transfer primary ownership |

### Workspace Permissions

| Permission | Description |
|---|---|
| `ws:read` | View workspace and its contents listing |
| `ws:write` | Edit workspace settings |
| `ws:create` | Create new workspaces (checked against org) |
| `ws:delete` | Delete workspace |

### Environment Permissions

| Permission | Description |
|---|---|
| `env:read` | View environment and list its connections |
| `env:write` | Edit environment settings |
| `env:create` | Create new environments (checked against workspace) |
| `env:delete` | Delete environment |
| `env:deploy` | Deploy changes to environment |

### Connection Permissions

| Permission | Description |
|---|---|
| `conn:metadata` | View connection name, type, host (not secrets) |
| `conn:read` | View connection secrets (password, connection string) |
| `conn:write` | Edit connection configuration |
| `conn:create` | Create new connections |
| `conn:delete` | Delete connection |
| `conn:execute` | Execute queries through the app (app proxies, user never sees credentials) |

### Query Permissions

| Permission | Description |
|---|---|
| `query:read` | View saved queries |
| `query:write` | Create and edit queries |
| `query:delete` | Delete queries |
| `query:execute` | Run queries |
| `query:share` | Change query visibility from private to shared |

### Job Permissions

| Permission | Description |
|---|---|
| `job:read` | View jobs and execution history |
| `job:write` | Create and edit jobs |
| `job:delete` | Delete jobs |
| `job:execute` | Manually trigger job execution |

### File Permissions

| Permission | Description |
|---|---|
| `file:read` | View files |
| `file:write` | Create and edit files |
| `file:delete` | Delete files |

### Dashboard Permissions (Future)

| Permission | Description |
|---|---|
| `dashboard:read` | View dashboards |
| `dashboard:write` | Create and edit dashboards |
| `dashboard:delete` | Delete dashboards |
| `dashboard:share` | Share dashboards with others |

### Policy Management Permissions (Enterprise)

| Permission | Description |
|---|---|
| `policy:read` | View role bindings and permission bindings |
| `policy:modify` | Create, edit, delete bindings (within anti-escalation rules) |

---

## 7. Role Model

### Builtin Roles

Three predefined roles ship with every organization. They cannot be modified or deleted. They are org-scoped — assigned at the organization level and grant permissions across all resources within the org.

#### Owner

Full control of the organization and all resources within it. Can perform destructive actions (delete org, delete workspaces, delete connections). Can manage members, assign roles, and transfer ownership.

#### Admin

Can manage infrastructure (create/edit workspaces, environments, connections), invite members, view connection secrets, and view all query history. Cannot perform destructive actions, cannot assign roles, cannot change org settings.

#### Member

Can use the infrastructure set up by admins. Can view workspaces, environments, and connection metadata. Can execute queries, write and save their own queries, share queries, and view their own query history. Cannot create or modify infrastructure. Cannot view connection secrets.

### Custom Roles (Enterprise Only)

Enterprise customers can create additional roles scoped to any resource type. Custom roles contain an explicit list of permissions and are assigned to specific resources.

### Role Scoping

A role's `scope_type` determines where it can be **assigned** (which resource type it binds to). A role can contain permissions for its own scope and any descendant scope in the hierarchy. A role cannot contain permissions for ancestor scopes.

| Scope Type | Can Contain Permissions For |
|---|---|
| `org` | `org:*`, `ws:*`, `env:*`, `conn:*`, `query:*`, `job:*`, `file:*`, `dashboard:*` |
| `workspace` | `ws:*`, `env:*`, `conn:*`, `query:*`, `job:*`, `file:*`, `dashboard:*` |
| `environment` | `env:*`, `conn:*` |
| `connection` | `conn:*` |

This means an environment-scoped role assigned to a specific environment can contain both `env:*` and `conn:*` permissions. The `conn:*` permissions in that role apply to all connections within that environment. See Section 8 for the rationale.

### Role Validation

When creating or editing a custom role, the service layer validates that every permission in the role is allowed for the role's `scope_type`. A connection-scoped role cannot contain `env:write`. An environment-scoped role cannot contain `ws:delete`.

---

## 8. Mixed-Scope Roles — No Implication Map

### Design Decision

Roles contain permissions for their own scope AND descendant scopes explicitly. There is no implication map that derives child permissions from parent permissions.

### What Was Considered and Rejected

An implication map was initially designed where roles would be strictly scoped (an environment role contains only `env:*` permissions) and a separate mapping table would derive child permissions (e.g., `env:write` implies `conn:read` on child connections). This was rejected for the following reasons:

**Two sources of truth.** If an environment role can contain `conn:read` explicitly AND the implication map also derives `conn:read` from `env:write`, there are two paths to the same permission. Audit becomes ambiguous — you cannot tell if access was explicitly granted or derived.

**Revocation is unreliable.** An admin removes `conn:execute` from a custom role thinking they've revoked query execution access. But the role still has `env:deploy`, and the implication map derives `conn:execute` from `env:deploy`. The team can still execute queries. The admin is confused.

**Custom role creation is confusing.** When creating a custom role, the admin doesn't know what the implication map provides. They check every permission "just to be safe," creating redundant entries. Or they miss permissions thinking the implication map covers it.

**Added complexity for no practical benefit.** The implication map is architecturally elegant but adds a second system (implication derivation) to explain the first system (roles). For a SQL IDE with a small, stable permission set, the simplicity of explicit permissions outweighs the convenience of derivation.

### How Mixed-Scope Roles Work

An environment-scoped role assigned to `environment:staging` with permissions `[env:read, env:write, conn:read, conn:execute, conn:metadata]` means:

- The assignee has `env:read` and `env:write` on `environment:staging`.
- The assignee has `conn:read`, `conn:execute`, and `conn:metadata` on **every connection** within `environment:staging`.

The hierarchy walk (see Section 10) finds the environment-level binding when checking permissions on any child connection. The role already contains the connection permissions — no derivation needed.

### What You See Is What You Get

The role definition is the complete truth. No hidden derivation. No implicit grants. If a permission is not in the role, it is not granted. This makes audit, debugging, and custom role creation straightforward.

### Tradeoff: Adding New Permissions

When a new permission like `conn:export` is added to the product, it must be added to every builtin role that should include it. With an implication map, adding one entry (`env:write → conn:export`) would propagate automatically. Without it, a migration updates the builtin role definitions. This is acceptable because:

- The permission set is small and stable.
- New permissions are added infrequently.
- Updating five builtin roles is a one-line migration.
- Custom roles are edited by admins who choose which new permissions to include.

---

## 9. Multiple Owners

### Design Decision

Multiple users and teams can hold the Owner role on the same resource. There is no artificial cap on the number of owners.

### Invariant: Minimum One Owner

Every organization and workspace must have at least one active owner at all times. This is enforced by the service layer, not the schema.

- A user cannot remove themselves as owner if they are the last owner.
- A user cannot demote the last owner to a lower role.
- The SCIM deprovision cascade (Section 18) transfers ownership before deactivating the last owner.

### No Maximum

Multiple owners is healthy. It prevents orphan situations where a single owner leaves and the resource becomes inaccessible.

### Primary Owner (Deferred)

Some products distinguish between a "primary owner" (one person, controls billing and org deletion) and "co-owners" (full admin permissions). This is deferred. Starting with equal owners is simpler and covers the majority of use cases. The primary owner concept can be added later when billing enters the picture by adding a `primary_owner_id` column to the `organizations` table.

---

## 10. Permission Resolution and Enforcement

### The Enforcer

The enforcer is the single function that answers "can this user do this action on this resource?" It is used by every mutation endpoint.

### Resolution Algorithm

```
Can(userID, permission, resourceType, resourceID):

1. Space check: if the current context is a Space, return true
   (user owns everything in their Space).

2. Author bypass: if this is an authored resource type (query, job, file)
   and the user is the author, return true.

3. Resolve principals: get the user's identity as a set of principals
   (their user ID + all team IDs they belong to in this org).
   Cached in the session.

4. Get ancestry: walk from the resource up through the hierarchy
   to the org root. e.g., for a connection:
   [connection:X, environment:Y, workspace:Z, org:W]

5. At each level of the ancestry, for each principal:
   a. Look up role bindings and permission bindings at this level.
   b. Expand role bindings into permission sets.
   c. Check if the requested permission is in the set.
   d. If found, return true.

6. If no level grants the permission, return false.
```

### Additive Union

Permissions are unioned across all levels and all principals. If a user has `conn:metadata` from their org-level member role and `conn:execute` from a workspace-level binding on their team, they have both. Permissions only add; they never subtract or override.

### Anti-Privilege Escalation

When granting permissions (creating role bindings or permission bindings), the service layer verifies that the granting user possesses every permission they are granting. A user cannot grant `conn:execute` if they don't have `conn:execute` themselves. This is checked at the service layer, not in the enforcer.

---

## 11. Performance — Batch Permission Resolution

### The Problem

List endpoints return many resources, each with a `permissions[]` array for the frontend to show/hide UI elements. Calling the enforcer per-resource per-permission would result in hundreds of permission checks for a single page load.

### Two Resolution Paths

**Mutation path (single resource):** Uses the enforcer's `Can()` function. One resource, one permission check. Hierarchy walk with role expansion. Sub-millisecond with warm cache.

**List path (batch resolution):** Bypasses the enforcer. One SQL query collects all role bindings and permission bindings for the user's principals at every ancestor level, then computes effective permissions for all resources at once.

### PostgreSQL Batch Query

For PostgreSQL deployments, a single query joins role bindings across all hierarchy levels and returns every resource with its effective permissions. The query unions bindings from connection-level, environment-level, workspace-level, and org-level, then filters to only the relevant permission prefixes per resource type.

### SQLite Batch Resolution

For SQLite deployments, 3-4 simple queries fetch all role bindings, all target resources, and the resource hierarchy. Permission computation happens in application code — iterating over resources and checking collected bindings against each level. For desktop-scale data (few workspaces, few connections), this runs in under a millisecond.

### Caching Strategy

| Data | Cache Strategy | Invalidation |
|---|---|---|
| Resource ancestry | Per-resource cache | On resource create/move/delete (rare) |
| User's principals (user + team IDs) | Session cache | On team membership change |
| Role bindings | In-memory enforcer model | On policy change (Redis pub/sub for multi-instance) |

---

## 12. SQLite Support

SQLite is supported for Desktop edition and small self-hosted Community deployments. The core principle is: **the database is just storage; the intelligence is in Go.**

### What Differs Between PostgreSQL and SQLite

| Feature | PostgreSQL | SQLite | Impact |
|---|---|---|---|
| Batch permission query | Complex single query | 3-4 simple queries + Go computation | None at desktop scale |
| Cache invalidation (multi-instance) | Redis pub/sub or LISTEN/NOTIFY | Not needed (single process) | None |
| Row-Level Security | Defense-in-depth layer | Absent, other layers sufficient | None for desktop |
| Array columns | `TEXT[]` native | JSON text, deserialized in Go | Minor |
| UUID generation | `gen_random_uuid()` | Generated in Go before insert | Trivial |
| Concurrent writes | MVCC, row-level locks | WAL mode, database-level lock | Fine for desktop |

### Abstraction Boundary

The repository interface is the abstraction point. Each repository method has a single interface with potentially two implementations (PostgreSQL and SQLite), selected via build tags. For 90% of repository methods, the SQL is identical between both databases (Bun handles dialect differences). Only the batch permission list query and a few edge cases differ.

### No PostgreSQL-Specific Features in the Core Model

- The implication map does not exist (removed by design).
- LISTEN/NOTIFY is not needed for single-process deployments. Multi-instance deployments use Redis pub/sub, not PostgreSQL-specific features.
- RLS is a bonus defense-in-depth layer for PostgreSQL but not required. Layers 1-4 (auth middleware, enforcer, service layer org check, repository WHERE clause) are sufficient.

---

## 13. Community Edition Defaults

### Permission Matrix

Community ships with three builtin org-level roles. All access is org-wide. No per-resource granularity.

| Permission Area | Owner | Admin | Member |
|---|---|---|---|
| View org settings | ✅ | ✅ | ❌ |
| Edit org settings / Delete org | ✅ | ❌ | ❌ |
| Invite/remove members | ✅ | ✅ | ❌ |
| Assign roles | ✅ | ❌ | ❌ |
| View workspaces/environments | ✅ | ✅ | ✅ |
| Create/edit workspaces/environments | ✅ | ✅ | ❌ |
| Delete workspaces/environments | ✅ | ❌ | ❌ |
| View connection metadata | ✅ | ✅ | ✅ |
| View connection secrets | ✅ | ✅ | ❌ |
| Create/edit/delete connections | ✅ | ✅/✅/❌ | ❌ |
| Execute queries | ✅ | ✅ | ✅ |
| Write/save/share queries | ✅ | ✅ | ✅ |
| View all query history | ✅ | ✅ | ❌ |
| Delete others' queries | ✅ | ✅ | ❌ |

### Design Rationale

- **Members can execute queries** because that's the core product value.
- **Members can't see connection secrets** because the app proxies connections.
- **Members can't create infrastructure** because that's admin work.
- **Admins can't delete** to prevent accidental destruction.
- **Admins can invite but can't assign roles** to prevent privilege escalation.

### What the Community Database Looks Like

The database schema is identical to enterprise. Community just has less data:

- The `organizations` table has real org records.
- The `roles` table has the three builtin roles.
- The `role_bindings` table has org-level bindings for each member.
- The `permission_bindings` table is empty (community never writes per-resource bindings).
- Custom roles don't exist (no UI to create them).
- Audit log is empty (community stubs don't log).

---

## 14. Enterprise Edition Features

Enterprise activates the following on top of community:

- **Per-resource permission bindings:** Grant specific permissions to specific users/teams on specific environments, connections, workspaces.
- **Custom roles:** Create reusable named permission bundles scoped to any resource type.
- **Policy management UI:** Full page for viewing, creating, editing, and deleting role bindings and permission bindings.
- **SCIM provisioning:** Automated user and team lifecycle management from an external IdP.
- **SAML SSO:** Per-org SAML authentication in addition to OIDC.
- **Full audit log:** Every policy change, access event, and admin action is logged.
- **JIT access requests:** Users can request time-bound access to resources they don't have.
- **Approval-based connection access:** Connections can require explicit approval before use.
- **Designated approvers:** Specific users or teams designated as approvers per resource.
- **Configurable session policies:** Per-org session timeout, SSO re-verification windows.

---

## 15. Community to Enterprise Upgrade Path

### Same Database, Zero Migration

Deploying the enterprise binary (or flipping a license key flag) activates enterprise features. The database is already enterprise-ready because the schema is identical.

- Existing org-level role bindings continue working.
- Enterprise routes for policy management, custom roles, SCIM, SAML, and audit become live.
- Admins can start creating per-resource bindings on top of existing org-level access.
- No data migration required.

### The Additive Nature

Enterprise doesn't replace community permissions — it adds more specific ones alongside them. The permission model is additive union. An admin upgrades and can gradually tighten access:

- Day 1: Everything works as before. No one notices.
- Day 7: Admin creates per-resource bindings, demotes some users from org-level admin to member, assigns specific environment/connection roles.
- Day 14: Admin sets up SCIM, teams sync from IdP.
- Day 30: Full enterprise RBAC in place, gradual transition complete.

### Downgrade

If a customer reverts to community, the community binary deploys. Enterprise routes become stubs. Per-resource bindings and custom roles still exist in the database but are never evaluated by new bindings (no UI to create them). Existing per-resource bindings still resolve and only add access (additive union means orphaned enterprise bindings grant more access, never less). Optionally, a cleanup script can remove non-org-level bindings on downgrade.

---

## 16. Session Management

### Stateful Sessions, Not JWTs

The SQL IDE uses server-side stateful sessions. JWTs are rejected for the following reasons:

- **Instant revocation required.** SCIM deactivates a user and their sessions must die immediately. JWTs continue working until expiry.
- **Session size.** The session carries org context, team memberships, SSO verification timestamps, and potentially step-up auth state. This grows beyond what fits comfortably in a request header.
- **Connection lifecycle.** The app holds persistent database connections on behalf of users. Sessions need server-side state to link to connection pools for cleanup on session death.

### Session Store

**Redis** is the primary session store for Kubernetes deployments. Sub-millisecond lookups on every request, built-in TTL expiry, pub/sub for real-time session invalidation across replicas.

**PostgreSQL-backed sessions** are the fallback for small community deployments that don't want Redis. Slower but functional.

**In-memory sessions** for desktop edition. Single process, single user, no persistence needed.

All three implement the same `SessionStore` interface.

### Session Object

The session contains:

| Field | Description |
|---|---|
| `ID` | High-entropy random string |
| `UserID` | The authenticated user |
| `Email` | User's email |
| `Status` | `active` or `step_up_required` |
| `ActiveOwnerType` | `space` or `org` |
| `ActiveOwnerID` | User ID (if space) or Org ID (if org) |
| `OrgMemberships` | All org memberships for the switcher UI |
| `TeamIDs` | Map of org_id → team IDs, cached for RBAC |
| `AuthMethod` | `oidc`, `saml`, `password`, `api_token` |
| `AuthenticatedAt` | When the user logged in |
| `IdPSessionID` | For OIDC/SAML back-channel logout |
| `MFAVerified` | Whether MFA was completed |
| `SSOVerifications` | Map of org_id → last SSO verification timestamp |
| `CreatedAt` | Session creation time |
| `LastActiveAt` | Updated on every request |
| `ExpiresAt` | Absolute session expiry |

### Session Lifecycle

**Login:** User authenticates → resolve user record → resolve all org memberships → resolve team memberships per org → create session in Redis with TTL → set HttpOnly/Secure/SameSite=Lax cookie → default active context is personal Space.

**Org/Space Switch:** User selects context from switcher → validate membership (or ownership for Space) → check SSO re-verification if needed → update active context in session → refresh team memberships for new org → frontend reloads.

**Every Request:** Read session cookie → lookup in Redis → check not expired → check idle timeout → update `LastActiveAt` → inject `OwnerContext` into request context.

**Logout:** Delete session from Redis → clear cookie → optional OIDC/SAML back-channel logout to IdP.

**Deprovision:** Deactivate user → delete all sessions for user across all orgs → user's next request gets 401.

### Timeout Configuration

| Timeout | Description | Default |
|---|---|---|
| Idle timeout | No activity for this duration → session expires. Reset on every request. | 30 minutes |
| Absolute timeout | Session expires after this duration regardless of activity. Forces re-authentication. | 24 hours |

Enterprise customers can configure these per org. Desktop has no timeout.

---

## 17. Multi-Org Login and SSO Re-verification

### Login Entry Points

**Instance login** (`app.example.com`): Instance-level authentication (email/password or instance-configured SSO). On success, session is created, user lands in their personal Space.

**Org-specific login** (`app.example.com/orgs/:orgID`): Redirects to the org's SSO provider if configured. On success, session is created with active context set to that org.

**Existing session**: If the user already has a valid session, they land in their last active context.

### Per-Org SSO Re-verification (Option B)

When a user switches to an SSO-protected org, the system checks whether the user has verified against that org's IdP within the configured re-auth window.

**First switch to an org:** Always triggers the OIDC/SAML flow. If the user has an active IdP session, this is seamless (redirect → callback → done). If not, they see the IdP login page.

**Subsequent switches within the re-auth window:** Seamless, no re-authentication needed.

**After the re-auth window expires:** Triggers the OIDC/SAML flow again on next switch.

### Re-auth Window Configuration

The re-auth window is configurable per org by the org admin, or controlled by the IdP's session policy. Default is 8 hours. Enterprise customers can set it to match their workday or compliance requirements.

The session stores per-org SSO verification timestamps. On org switch, the middleware checks: `time.Since(session.SSOVerifications[orgID]) < org.SSOReauthWindow`.

### Personal Space

The personal Space never requires SSO. It uses instance auth. If the user is logged in at all, their Space is accessible. This is the fallback — if they're removed from all team orgs, they still have their Space.

---

## 18. Ownership Deprovision Cascade

### The Problem

When SCIM deactivates the sole owner of an organization or workspace, the resource becomes orphaned — nobody can manage it, change settings, transfer ownership, or delete it.

### Design Decision: Accept the Deprovision, Auto-Cascade Ownership

When SCIM sends a delete/deactivate for a user, the system deactivates the user but first runs the ownership cascade.

### Cascade Logic

For each resource where the deprovisioned user is the sole owner:

1. **Another owner exists?** Do nothing. Resource is safe.
2. **No other owner but admins exist?** Promote the longest-tenured admin to owner.
3. **No owners and no admins?** Promote the longest-tenured member to owner.
4. **No other members at all?** Flag the resource as orphaned for super_admin review.

### Successor Selection Priority

1. Another admin on the same resource (most contextually appropriate).
2. Any admin at the org level (fallback for workspace ownership).
3. Org owner (for workspace ownership).
4. Any active member (last resort).

### Bulk Deprovision Handling

When an IdP removes an entire team, SCIM sends multiple delete requests. These are processed **sequentially, not in parallel** per org. Each deprovision may change who the successor is for the next one. A per-org mutex or serial processing queue ensures consistency.

### Audit and Notification

Every auto-transfer is logged to the audit log with the reason ("previous owner deprovisioned via SCIM"), the promotion rule used, and the previous/new owner. The new owner receives an immediate notification.

### Orphaned Resources

When no successor exists (all members deprovisioned), the resource is flagged for super_admin review. The resource still exists, data is intact, but no one can access it until a super_admin assigns a new owner.

---

## 19. JIT Query Access

### Concept

"I need to run a query on prod-pg right now, but I don't have `conn:execute`." JIT (Just-In-Time) access allows users to request time-bound permissions on resources they don't currently have.

### The Access Request

An access request specifies:

- The resource (type + ID) the user wants access to.
- The permissions requested (e.g., `[conn:execute, conn:metadata]`).
- A reason / justification.
- A requested duration (e.g., 4 hours).

### Access Request Lifecycle

```
Pending → Approved → Active → Expired
               ↓
            Denied

Active → Revoked (manually by admin or approver)
```

### When Approved

An approved access request creates normal permission bindings with an `expires_at` timestamp. The enforcer's permission lookup includes a check: `WHERE expires_at IS NULL OR expires_at > now()`. No special enforcement path — time-bound bindings are just regular bindings that expire.

### Anti-Escalation on Approval

The approver must possess every permission they are approving. An approver with only `conn:metadata` cannot approve a request for `conn:execute`.

### Expiry Cleanup

A background job periodically finds expired bindings, notifies the user that their access expired, logs the event, and marks the bindings as expired (not deleted, for audit trail).

### Schema Additions

The `access_requests` table stores the request lifecycle. The `role_bindings` and `permission_bindings` tables gain `expires_at`, `source_type` (manual, access_request, auto_approve_rule), and `source_id` columns. These columns are added from day one even if JIT access is enterprise-only — the columns cost nothing and avoid a migration later.

### Edition Gating

JIT access is enterprise-only. Community has the `expires_at` column but no UI or API to create access requests.

---

## 20. Approval-Based Connection Access

### Concept

Separate from JIT access. Approval-based connection access means: "Before you can use this connection at all, even if you have the RBAC permission, an admin must explicitly approve your access."

### Connection Access Modes

Each connection has an `access_mode`:

- **`open` (default):** Anyone with the appropriate RBAC permission can use it.
- **`approval`:** RBAC permission is required AND an active connection approval record must exist.

### Enforcement

Approval is a **second gate** after RBAC, not a replacement. The request pipeline is:

```
Request → Auth → RBAC check → Approval check → Handler
```

Both must pass for approval-mode connections.

### Connection Approval Records

A `connection_approvals` table stores active approvals: which user is approved for which connection, who approved it, when it was approved, and when it expires (if time-bound).

### Combined with JIT

When someone requests JIT access to an approval-mode connection, the approval flow grants both the RBAC permission binding and the connection approval in one action. The user doesn't go through two separate approval flows.

### Edition Gating

Approval-based connection access is enterprise-only. Community connections always use `access_mode = 'open'`.

---

## 21. Designated Approvers

### Design Decision: Option C — Designated Approvers Per Resource

Rather than allowing anyone with `policy:modify` to approve (Option A) or anyone who already has the permission (Option B), specific users or teams are explicitly designated as approvers for each resource that requires approval.

### Storage

An `approver_designations` table maps resources to their approvers:

| Column | Description |
|---|---|
| `resource_type` | The resource type (e.g., `connection`) |
| `resource_id` | The specific resource |
| `approver_type` | `user` or `team` |
| `approver_id` | The designated approver |
| `owner_type` | `space` or `org` |
| `owner_id` | Owner context |

### Behavior

- Approvers can be individual users or teams.
- When a user requests access, all designated approvers for that resource are notified.
- Any single designated approver can approve or deny.
- The approver must still possess the permissions they are approving (anti-escalation).

### Deferred Details

The following are deferred for later design:

- "All must approve" vs "any one" approval modes.
- Approval escalation (if no one approves in N hours, escalate).
- Fallback approvers when all designated approvers are deprovisioned.
- Delegation (an approver delegates to someone else).
- Requester cancellation of pending requests.

---

## 22. Author Bypass and Resource Ownership

### Concept

Queries, jobs, files, and dashboards have an author — the user who created them. The author always has full control of their own resources regardless of RBAC.

### Implementation

Every authored resource table has an `author_id` column. The enforcer checks author ownership **before** RBAC resolution. If the user is the author, the check returns true immediately.

### Scope

The author bypass applies to:

- Queries: author can always read, edit, delete, and execute their own queries.
- Jobs: author can always read, edit, delete, and trigger their own jobs.
- Files: author can always read, edit, and delete their own files.
- Dashboards: author can always read, edit, and delete their own dashboards.

The author bypass does NOT apply to infrastructure resources (workspaces, environments, connections) — those are always governed by RBAC.

---

## 23. Private vs Shared Visibility

### Concept

Queries and files have a visibility setting orthogonal to RBAC:

- **Private:** Only the author can see it. RBAC is irrelevant. Even workspace admins cannot see private queries (unless a separate `query:view_private` permission is introduced for compliance, deferred).
- **Shared:** Visible to anyone with `query:read` on the workspace. RBAC controls who can edit or delete.

### Implementation

A `visibility` column (`'private'` or `'shared'`) on the queries and files tables. List endpoints filter:

- Return all resources where `author_id = current_user` (regardless of visibility).
- Return all resources where `visibility = 'shared'` AND user has the read permission.
- Never return other users' private resources.

### Sharing Model

Community uses visibility toggles only (private ↔ shared). Enterprise can add per-resource permission grants for more granular sharing (e.g., share a query with a specific team but not the whole workspace) using the existing permission binding mechanism.

---

## 24. Connection Secrets Separation

### The Critical Distinction

`conn:read` and `conn:execute` serve fundamentally different purposes:

- **`conn:metadata`:** Can see connection name, type, host. Not secrets.
- **`conn:read`:** Can see the actual credentials — password, connection string. Only admins who configure connections need this.
- **`conn:execute`:** Can run queries through the app. The app proxies the connection. The user never sees credentials.

### API Enforcement

Separate endpoints enforce the distinction:

- `GET /connections/:id` → requires `conn:metadata`. Returns name, host, type.
- `GET /connections/:id/secrets` → requires `conn:read`. Returns password, connection string.
- `POST /connections/:id/execute` → requires `conn:execute`. Proxied query execution.

Most users should have `conn:execute` + `conn:metadata`, not `conn:read`.

---

## 25. Audit Logging

### Concept

Every significant action is logged to an append-only audit trail. Audit logs are enterprise-only (community stubs return empty).

### Event Categories

- **Auth events:** Login, logout, failed login, SSO verification, session expiry.
- **RBAC events:** Role assigned, role removed, permission granted, permission revoked, custom role created/edited/deleted.
- **Access events:** Access request created, approved, denied, expired, revoked.
- **Resource events:** Resource created, updated, deleted, transferred between Space and Org.
- **Connection events:** Connection secrets viewed.
- **Admin events:** User invited, user deprovisioned, org settings changed, ownership transferred.

### Properties

- **Append-only.** No updates, no deletes. Immutability is critical for compliance.
- **Tamper-proof.** The database user for audit writes should have INSERT-only grants.
- **Exportable.** Enterprise customers ship audit logs to external SIEM systems.

### Deferred Details

Detailed audit schema (single table vs per-event-type tables), retention policies, export format, and SIEM integration are deferred for later design.

---

## 26. Query Execution Logging

### Separate from Audit Logging

Query execution logging tracks who ran what SQL against which connection. This is a high-volume operational log, separate from the policy audit trail.

### Fields

- Who: user ID, email
- What: SQL hash (not the full SQL for storage), SQL preview (first N characters), query ID (if saved query, NULL for ad-hoc)
- Where: connection ID, environment ID, workspace ID, org ID
- When: execution timestamp
- Result: row count, duration, status (success/error/timeout), error message
- Context: client IP, session ID

### Properties

- Append-only, same as audit logs.
- Enterprise customers use this for compliance ("who accessed customer data last month?").
- The combination of policy audit (who had access) and execution audit (who actually used it) answers the full compliance question.

### Deferred Details

Schema design, retention, and storage optimization are deferred.

---

## 27. Data Isolation and Cross-Org Security

### Five Layers of Isolation

1. **Auth middleware:** Every request must have a valid session with an active org/space context.
2. **Enforcer:** RBAC check confirms the user has the required permission in the current context.
3. **Service layer:** Every write operation validates `owner_type` and `owner_id` match the current context.
4. **Repository layer:** Every query includes `WHERE owner_type = ? AND owner_id = ?`.
5. **PostgreSQL RLS (bonus):** Row-level security policies filter on the current context. Not available on SQLite but not required — layers 1-4 are sufficient.

### Cross-Context Behavior

- A resource in Org A is invisible to users in Org B. Requests for resources in the wrong context return 404 (not 403) to avoid leaking existence.
- A resource in Space A is invisible to Space B.
- Cross-workspace references are not allowed within the same org/space. Workspaces are isolated containers.

### Connection Secret Encryption

Deferred: Connection strings stored encrypted at rest. Key management strategy (per-org encryption key vs instance-level key) is deferred.

---

## 28. Extensibility — Future Resource Types

### Adding a New Resource Type

Every new resource type follows the same pattern with no changes to RBAC tables, enforcer algorithm, or existing schema:

1. Create a table with `owner_type`, `owner_id`, and parent foreign key.
2. Add an entry in `resource_hierarchy`.
3. Define permission strings (`<type>:read`, `<type>:write`, etc.).
4. Add permissions to existing builtin role definitions (migration).
5. Optionally create new scoped roles for the resource type.
6. Done.

### Planned Future Resources

| Resource | Parent | Notes |
|---|---|---|
| Query | Workspace | Has `author_id`, `visibility`, `target_environment_id` |
| Job | Workspace | Scheduled query execution. Has `author_id`. |
| JobRun | Job | Execution history record. Read-only. |
| File | Workspace | SQL files, notebooks, docs. Has `author_id`, `visibility`. |
| Dashboard | Workspace | Query result visualization. Has `author_id`. |
| Panel | Dashboard | Individual chart/table within a dashboard. |

### Design Decision: Workspace-Level Only

Queries, jobs, files, and dashboards are workspace-level resources, not environment-level. If a query targets a specific environment, it stores a `target_environment_id` field, but its position in the hierarchy (and therefore its RBAC parent) is the workspace.

This avoids the complexity of resources that can be children of either a workspace or an environment.

### Job Execution Identity

Jobs run on a schedule with no user session. The initial design uses "run as creator" — the job stores the creator's ID and checks their permissions at execution time. If the creator is deprovisioned or loses access, the job fails and workspace admins are notified. Service accounts for job execution are deferred.

---

## 29. Error Messages and Permission Denial UX

### Cross-Context: Always 404

When a user requests a resource in a different org or space, the response is 404. Never 403. Never leak that a resource exists in another context.

### Within Context: Informative 403

When a user is denied within their own org, the response should include:

- Which permission they are missing.
- Which resource the check failed on.
- Who can grant them access (or a link to the access request form if JIT access is available).

### Deferred: Explain() Function

A sibling to the enforcer's `Can()` that returns the denial reason and suggested fix. Deferred for later design but the enforcer's architecture should accommodate it.

---

## 30. Pre-Implementation Checklist

### P0 — Gaps in Authored Resources

| Item | Action |
|---|---|
| `author_id` on queries, jobs, files | Add column, implement pre-RBAC author check in enforcer |
| Private vs shared `visibility` | Add column, filter in list endpoints |
| `conn:read` vs `conn:execute` API separation | Separate endpoints, enforce in handlers |

### P0 — Schema Additions for Future Features

| Item | Action |
|---|---|
| `expires_at` on role_bindings and permission_bindings | Add columns now, build JIT UI later |
| `source_type` and `source_id` on bindings | Add columns now for traceability |
| `access_mode` on connections | Add column, default `'open'` |

### P1 — Time-Bound Access Infrastructure

| Item | Action |
|---|---|
| `access_requests` table | Design and create |
| `connection_approvals` table | Design and create |
| `approver_designations` table | Design and create |
| Expiry cleanup background job | Implement as CronJob or in-process ticker |

### P1 — Session and Auth Infrastructure

| Item | Action |
|---|---|
| Redis session store | Implement `SessionStore` interface |
| PostgreSQL fallback session store | Implement `SessionStore` interface |
| In-memory session store (desktop) | Implement `SessionStore` interface |
| Per-org SSO verification tracking | Add `SSOVerifications` to session |
| Session idle and absolute timeout | Implement in middleware |

---

## 31. Deferred TODO Items

The following items were identified during design but are deferred for later. They are organized by priority.

### P0 — Must Design Before Implementation

| Item | Notes |
|---|---|
| **Instance admin model** | How the first admin is created (bootstrap problem). What instance admins can do. Whether they can access resources inside orgs they're not members of. Whether instance admin is in the RBAC system or separate. |
| **Authentication flow details** | OIDC flows (Authorization Code + PKCE). Password storage (bcrypt vs argon2). Password reset flow. Email verification. Account lockout. Instance SSO vs org SSO coexistence. SAML SP-initiated vs IdP-initiated. Back-channel logout. |
| **API token design** | Scoping (full user permissions vs subset). Format (prefix + random, e.g., `sqli_live_abc123`). Storage (hash only, user sees token once). Per-org vs cross-org. Expiry. Rotation. Rate limits. Interaction with org SSO requirements. |
| **Invitation flow** | Invite by email vs link. Pre-assigned role on invite. Account creation on invite acceptance. Invite expiry. Interaction with SCIM (manual invites disabled for SCIM orgs?). Rate limiting. |
| **Data isolation enforcement details** | Resource ID guessability with numeric IDs. `owner_type`/`owner_id` validation on every write vs middleware + WHERE clause. Connection string encryption at rest. Per-org vs instance-level encryption keys. |

### P1 — Must Design Before Beta

| Item | Notes |
|---|---|
| **Designated approver edge cases** | "All must approve" vs "any one." Escalation after N hours. Fallback when all approvers deprovisioned. Delegation. Requester cancellation. |
| **Per-org SSO re-verification edge cases** | What happens during re-auth redirect (user loses place in app?). Grace period if IdP is down. Whether re-auth window resets on activity or is absolute. |
| **Audit log schema** | Single table vs per-event-type. Retention policy. Immutability enforcement. Export format. SIEM integration. |
| **Concurrent session and connection cleanup** | Kill DB connections on session expiry. Kill running queries on deprovision. Track which sessions hold which DB connections. Connection pooling strategy. Max concurrent connections per user/org. |
| **Data ownership on user removal from org** | Saved queries — transfer or orphan? Private queries — delete or convert? Running jobs — fail or reassign? Execution history — keep for audit. |
| **Frontend permission refresh** | How often frontend refreshes permissions (page nav, polling, websocket). Friendly error on 403 from stale permissions. Whether to kill running query if user loses `conn:execute` mid-session. |

### P2 — Must Design Before Enterprise Launch

| Item | Notes |
|---|---|
| **SCIM implementation** | Which endpoints (/Users, /Groups). Groups → Teams mapping. Conflict resolution (SCIM user already exists from manual invite). Attribute mapping. Patch operations. Token rotation. Error handling. |
| **Team management** | CRUD for teams. Manual add/remove vs SCIM group sync. Nested teams (probably no for v1). Team deletion behavior. |
| **Custom role lifecycle** | Who can create custom roles. Org-scoped custom roles (powerful, dangerous). Deleting a role that's assigned. Role versioning (edit takes effect immediately?). Limits per org. Name uniqueness. |
| **Resource limits and quotas** | Limits per Space. Limits per org by plan tier. Storage (hardcoded vs configurable). Soft block vs hard block. Instance admin override per org. |

### P3 — Design When Customers Ask

| Item | Notes |
|---|---|
| Temporary/time-bound access UI | Columns exist, UI deferred. |
| IP allowlisting / conditional access | Middleware slot should exist from day one. |
| MFA step-up for sensitive operations | Which permissions trigger step-up. |
| Webhooks for policy change events | Build on audit log event stream. |
| Cross-workspace resource linking | Keep workspaces isolated in v1. |
| Auto-approve rules for JIT access | e.g., "staging access auto-approves for 4 hours." |
| Approval escalation chains | Multi-level approval routing. |
| RBAC analytics |
