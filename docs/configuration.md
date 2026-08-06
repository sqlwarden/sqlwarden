# Configuration

This reference applies to the SQLWarden server.

SQLWarden separates deployment-managed bootstrap configuration from database-backed runtime settings.

Bootstrap configuration is read from defaults, config files, environment variables, and CLI flags. It is validated before listeners and workers start, and changes require a restart. Runtime settings are stored in the application database and changed through the administration API. They are read directly from the database for each request or background operation.

Bootstrap loading is implemented in `internal/web/config.go`. Runtime ownership and inheritance are implemented by the runtime settings service and typed database models.

## Configuration Lifecycle

The following settings remain bootstrap-only:

- HTTP listener, deployment mode, access mode, and log format.
- Application database driver, DSN, and startup migration behavior.
- Cookie, JWT-signing, and encryption keys.
- TLS certificate configuration.
- File-storage mode, active backend, backend definitions, and filesystem roots.
- Desktop backend topology.
- Allowed host-local SQLite sources.

Changing a bootstrap setting requires a restart. Changing the application DSN selects another SQLWarden instance database; changing storage configuration does not move stored files. Secret rotation must use the documented key/session rotation behavior. SQLWarden does not perform these migrations automatically.

The following settings are database-backed at runtime:

- Instance identity, public URL, support email, and personal spaces.
- JWT access-token lifetime and session revocation.
- Interactive query and export limits.
- Schema snapshot freshness.
- File revision policy.
- Error-notification recipient.
- Log level and application database query tracing.
- Job worker count, polling, claim lease, and completed-job retention.
- SMTP enablement, connection details, sender, and encrypted password.

Query/export limits, snapshot freshness, and file revision policy have nullable organization overrides. An organization may only tighten the instance policy: lower limits, a longer snapshot interval, or fewer revisions. Personal workspaces use instance settings.

Operational settings are applied immediately on the process that accepts the update and are reconciled by every other replica from the shared application database.

## Bootstrap Sources

Configuration is applied in this order:

1. Built-in defaults.
2. `sqlwarden.*` and `.sqlwarden.*` files from the current directory or `./config`.
3. Environment variables.
4. CLI flags.

You can pass an explicit config file:

```sh
sqlwarden --config /etc/sqlwarden/sqlwarden.yaml
```

Supported config file formats are YAML, JSON, and TOML.

Environment variables use uppercase snake case. Nested config keys replace dots with underscores. For example, `db.dsn` becomes `DB_DSN`.

CLI flags use kebab case. For example, `db.dsn` becomes `--db-dsn`.

To print available flags:

```sh
sqlwarden --help
```

## Minimal Config File

```yaml
base_url: http://localhost:6020
http_port: 6020
log:
  level: info
  format: json

db:
  driver: sqlite
  dsn: ~/.sqlwarden/sqlwarden.db
  automigrate: true

cookie:
  secret_key: replace-with-a-random-secret

jwt:
  secret_key: replace-with-a-random-secret

encryption:
  key: replace-with-a-random-secret

files:
  root_dir: ~/.sqlwarden/files

jobs:
  worker_count: 16
  poll_interval: 1s
  claim_lease: 5m
  completed_retention: 168h

```

## Docker Example

Released images are published to GitHub Container Registry.

```sh
docker run --rm \
  --name sqlwarden \
  -p 6020:6020 \
  -v sqlwarden-data:/var/lib/sqlwarden \
  -e BASE_URL=http://localhost:6020 \
  -e DB_DSN=/var/lib/sqlwarden/sqlwarden.db \
  -e FILES_ROOT_DIR=/var/lib/sqlwarden/files \
  -e COOKIE_SECRET_KEY=replace-with-a-random-secret \
  -e JWT_SECRET_KEY=replace-with-a-random-secret \
  -e ENCRYPTION_KEY=replace-with-a-random-secret \
  ghcr.io/sqlwarden/sqlwarden:0.6.1
```

The default image runs as the `sqlwarden` user. The volume path above persists the SQLite database and file storage directory.

## Server

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `base_url` | `BASE_URL` | `--base-url` | `http://localhost:6020` | Public base URL used for generated links and JWT claims. |
| `http_port` | `HTTP_PORT` | `--http-port` | `6020` | HTTP server port. |

## Logging

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `log.format` | `LOG_FORMAT` | `--log-format` | `json` | Server log format. Supported values: `json`, `text`. |

JSON logs are the default for production and log aggregation systems. Text logs are intended for local development.
The log level is an instance runtime setting and changes live without restarting the server.

Every HTTP response includes `X-Request-ID`. If the request provides a valid bounded `X-Request-ID`, SQLWarden preserves it; otherwise it generates one. Access logs include request ID, route, path, response status, duration, remote IP, user agent, and resolved account/resource identifiers when available.

Server logs include request-aware operational events for authentication, authorization failures, resource mutation, database engine capability lookup, schema inspection, live database sessions, and query cursor lifecycle. `debug` enables lower-level diagnostics such as capability resolution and schema response summaries.

Server logs do not include request bodies, authorization headers, DSNs, SQL text, bind parameters, raw query strings, or row values by default.

## Database

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `db.driver` | `DB_DRIVER` | `--db-driver` | `sqlite` | Application database driver. Supported values: `sqlite`, `postgres`. |
| `db.dsn` | `DB_DSN` | `--db-dsn` | `~/.sqlwarden/sqlwarden.db` | SQLite path or PostgreSQL DSN. `~` is expanded for SQLite. |
| `db.automigrate` | `DB_AUTOMIGRATE` | `--db-automigrate` | `true` | Runs embedded migrations at startup. |

PostgreSQL DSNs are passed without a `postgres://` prefix in the existing compose setup:

```sh
DB_DRIVER=postgres
DB_DSN=sqlwarden:sqlwarden_password@localhost:5432/sqlwarden?sslmode=disable
```

SQLite is the default because it gives local and small self-hosted deployments a zero-dependency start:

```sh
DB_DRIVER=sqlite
DB_DSN=~/.sqlwarden/sqlwarden.db
```

Use PostgreSQL for larger deployments, environments with multiple server replicas, or environments where operational policy requires a managed database.

## Secrets And Sessions

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `cookie.secret_key` | `COOKIE_SECRET_KEY` | `--cookie-secret-key` | Development-only secret | Cookie signing secret. Replace in every real deployment. |
| `jwt.secret_key` | `JWT_SECRET_KEY` | `--jwt-secret-key` | Development-only secret | JWT signing secret. Replace in every real deployment. |
| `encryption.key` | `ENCRYPTION_KEY` | `--encryption-key` | Development-only secret | Application encryption key for encrypted values such as DSNs and SMTP credentials. Replace in every real deployment. |
| `encryption.previous_keys` | `ENCRYPTION_PREVIOUS_KEYS` | `--encryption-previous-keys` | Empty | Comma-separated retired encryption keys retained for decrypting old ciphertext during rotation. |

Do not use the default secrets outside local development.

## Interactive Queries

Interactive query limits are runtime settings managed at instance or organization scope through the administration API.

The same limits apply to HTTP query cursors. Direct `/query` responses are capped once per response. Query-cursor start and fetch responses are capped per page; clients can continue fetching while the response has `exhausted=false`.

## Schema Snapshots

Persistent schema snapshots are enabled by default per organization and can be
disabled in organization settings. A connection may inherit that setting or
tighten it to `disabled`; a connection cannot override a disabled organization.
Disabling snapshots immediately deletes stored schema metadata. In that mode,
schema inspection uses only the process-local cache associated with a live
database session.

For DQL/select-style queries, the IDE can request cursor-backed results through `/query`. When the selected target engine supports cursor-backed results, the first response includes the first page plus cursor metadata. Engines that do not support cursor-backed results fall back to the bounded direct query path. Cursor state is process-local and tied to the authenticated live database session; it is not durable query history.

## Exports

Export limits are runtime settings managed at instance or organization scope through the administration API. Synchronous exports use the caller's existing live database session and stop if the HTTP request is cancelled. Background exports run as user-visible jobs, open their own short-lived target database connection, and write output to private workspace files.

## Background Jobs

Worker count, polling interval, claim lease, and completed-job retention are instance runtime settings. Updates restart only the in-process job runner; the API process stays available.

Jobs are persisted in the application database. Workers always run inside the API process and use database claim leases so a future separate worker binary can use the same job table safely. Job scheduling is best effort: due jobs run when a worker is available, with higher-priority due jobs claimed before lower-priority due jobs. Internal maintenance such as stale file-content cleanup uses this framework.

Maintenance jobs that must have only one active instance use a database-enforced singleton key, so multiple API processes can safely race to schedule the same maintenance work in distributed deployments.

User-facing jobs can also persist progress events. Events are read through the scoped job API with an `after_id` marker so clients can poll only for new events. Events follow the parent job retention period; there is no separate event retention setting.

The claim lease is stale-worker recovery time, not a maximum job runtime. Running jobs heartbeat to extend the lease while the worker is healthy.

## TLS

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `tls.enabled` | `TLS_ENABLED` | `--tls-enabled` | `false` | Serves HTTPS directly from SQLWarden. |
| `tls.cert_file` | `TLS_CERT_FILE` | `--tls-cert-file` | Empty | PEM certificate path. Required when TLS is enabled. |
| `tls.key_file` | `TLS_KEY_FILE` | `--tls-key-file` | Empty | PEM private key path. Required when TLS is enabled. |

Many deployments should terminate TLS at a reverse proxy. Built-in TLS is available when direct HTTPS serving is preferred.

## Files

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `files.root_dir` | `FILES_ROOT_DIR` | `--files-root-dir` | `~/.sqlwarden/files` | Filesystem root directory for file content. `~` is expanded. |
The server stores workspace file content on the local filesystem by default. Revision policy is a database-backed runtime setting. The storage implementation has internal backend plumbing for future expansion.

## Target SQLite Connections

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `drivers.sqlite.allowed_sources` | `DRIVERS_SQLITE_ALLOWED_SOURCES` | `--drivers-sqlite-allowed-sources` | Empty | Comma-separated SQLite target sources to allow. Currently supports only `local`. |

PostgreSQL and MySQL target connections are available through the normal connection flow.

SQLite target connections are explicitly gated because local SQLite paths can expose host-local files. Server deployments should leave this empty unless they intentionally allow local SQLite access.

To enable local SQLite target connections:

```sh
DRIVERS_SQLITE_ALLOWED_SOURCES=local
```

## Email

SMTP is optional and configured in instance runtime settings. The password is write-only through the API and encrypted with `ENCRYPTION_KEY` at rest. Configure and enable SMTP to deliver organization invitations and error notifications.

## Production Checklist

- Set `BASE_URL` to the public URL users will access.
- Replace `COOKIE_SECRET_KEY`, `JWT_SECRET_KEY`, and `ENCRYPTION_KEY`.
- Decide whether to use SQLite or PostgreSQL for the application database.
- Persist `~/.sqlwarden` or explicitly configure database and file storage paths.
- Keep `LOG_FORMAT=json` for production log collection.
- Leave database query tracing disabled unless actively debugging.
- Leave `DRIVERS_SQLITE_ALLOWED_SOURCES` empty unless local SQLite target access is intentional.
- Use HTTPS through a reverse proxy or SQLWarden built-in TLS.
