# Distribution Architecture

SQLWarden uses compile-time dependency injection to compose Community and paid distributions. The enterprise repository imports the public Community packages, constructs paid implementations, and passes one dependency object to the application composition root. Community does not import enterprise code and does not discover plugins at runtime.

This boundary is intentionally small and source-level. Community and enterprise are developed and released in lockstep. It is not a stable third-party plugin SDK and does not promise compatibility across mismatched versions.

## Dependency Direction

```text
enterprise command
  -> enterprise feature constructors
  -> github.com/sqlwarden/app
  -> github.com/sqlwarden/distribution
  -> Community internal implementation
```

The public packages used by a distribution are:

- `app`: application construction, configuration, serving, shutdown, and operational commands.
- `distribution`: typed dependencies that one trusted distribution can supply.
- `authorization`: the authorization request, decision, authorizer, and restrictive constraint contracts.
- `buildinfo`: Community and distribution artifact identity.

All Community implementation details remain under `internal/`. Enterprise code must not import them. The downstream contract test compiles an external module against only the public packages to enforce this boundary.

## Composition

An enterprise command constructs SQLWarden with `app.WithDistribution`. The configure function receives `distribution.HostServices` once and returns `distribution.Dependencies`.

Host services expose narrow operations for accounts, organizations, sessions, jobs, request context, logging, and the shared Bun database. The host object belongs in the enterprise composition root. Individual feature constructors should receive only the interfaces they use instead of retaining the complete host object.

Startup order is deterministic:

1. Validate Community configuration and initialize the application database.
2. Run Community migrations.
3. Construct Community infrastructure, RBAC, and job services.
4. Call the distribution configure function.
5. Register distribution permission metadata.
6. Run distribution migration sets using independent migration ledgers.
7. Reconcile additive defaults into existing builtin roles.
8. Compose the restrictive authorization constraint.
9. Register distribution jobs and routes.
10. Start the distribution lifecycle, Community workers, and HTTP server.

Shutdown stops Community workers, invokes the distribution lifecycle shutdown hook, closes live target database resources, and closes the application database.

## Authorization

Community RBAC remains the mandatory eligibility layer. A distribution can inject an `authorization.Constraint` that evaluates only requests Community already allowed. A constraint may allow the existing decision to stand or deny it with a stable code and user-facing message. It cannot turn a Community denial into an allow.

Constraint denials flow through the standard API error envelope, including connection establishment, query execution, query cursors, schema access, and exports. Enterprise frontends can therefore react to codes such as `approval_required` without Community importing approval-specific behavior.

This supports paid policy features such as approvals, JIT access, risk controls, and external authorization without replacing or bypassing core tenant isolation, membership gates, or RBAC.

Injected permissions extend the application-local permission catalog. They declare valid scopes, resource applicability, and optional additive defaults for existing Community builtin roles. Startup reconciliation inserts those defaults into roles in existing installations. Permission registration is per application instance and never mutates package-global state.

## Routes And Sessions

The distribution receives direct Chi router mounts for public, account, instance, organization, workspace, environment, and connection contexts. Except for the public mount, these routers already carry the corresponding Community authentication and resource-context middleware. Enterprise routes use product-native paths and can apply Community or enterprise middleware directly.

SSO and other external identity integrations should use the account, organization, and session services. After an external identity is verified, `SessionService.Complete` creates the same SQLWarden auth session, refresh token, cookie, and access token used by Community authentication. Enterprise code does not implement a parallel session model.

## Data And Jobs

Community and enterprise share the application database connection and transaction system. Each distribution migration set has its own migration ledger, which prevents Community and enterprise migration versions from colliding. Enterprise schema ownership remains in the enterprise repository.

Paid background work registers normal persisted job definitions. Jobs use the Community job store, workers, leases, retry behavior, and event timeline, so exports, report generation, SCIM synchronization, SIEM delivery, and future workflows survive process restarts consistently.

## Frontend Composition

Community exports a typed `FrontendDependencies` object with optional providers, routes, and navigation entries. The Community build uses an empty dependency object. An enterprise build points `SQLWARDEN_FRONTEND_DISTRIBUTION` at its own dependency module, and Vite aliases that module into the same React application.

Enterprise routes are attached to existing TanStack Router parents at build time. Navigation entries use the existing app shell and can be gated by effective permission strings. Provider composition supports enterprise state, licensing, telemetry, and identity contexts without forking Community root components. React and TanStack dependencies are deduplicated so both repositories use one runtime instance.

The external frontend contract build compiles a dependency module outside the Community source tree. This catches import, type, Tailwind source, and bundling regressions.

## Adding New Seams

Do not add a generic hook, event bus, module registry, feature manifest, or override map in anticipation of unknown paid features. When a real feature needs integration:

1. Identify the narrow Community decision or service boundary.
2. Define a typed interface around that boundary.
3. Keep the Community implementation as the default.
4. Inject an optional decorator, policy, or provider from the distribution composition root.
5. Add a contract test proving Community behavior remains intact and the paid implementation composes externally.

This keeps the open-core boundary explicit while allowing future approval workflows, JIT access, audit sinks, SSO, SCIM, observability, and SIEM integrations to add only the seams they actually require.
