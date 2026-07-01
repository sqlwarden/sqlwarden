# Configuration

This reference applies to the SQLWarden server.

SQLWarden reads configuration from defaults, config files, environment variables, and CLI flags. The same setting can usually be expressed in all three forms.

Configuration is implemented in `internal/web/config.go`. Treat that file as the source of truth when adding or changing runtime options.

## Sources

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
personal_spaces_enabled: true

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
  access_token_ttl: 24h

encryption:
  key: replace-with-a-random-secret

files:
  root_dir: ~/.sqlwarden/files
  revisions:
    enabled: true
    keep_latest: 50

query:
  max_result_rows: 10000
  max_result_bytes: 26214400

jobs:
  worker_count: 2
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
| `personal_spaces_enabled` | `PERSONAL_SPACES_ENABLED` | `--personal-spaces-enabled` | `true` | Enables account-owned personal spaces under `/api/v1/me`. |

## Logging

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `log.level` | `LOG_LEVEL` | `--log-level` | `info` | Server log level. Supported values: `debug`, `info`, `warn`, `error`. |
| `log.format` | `LOG_FORMAT` | `--log-format` | `json` | Server log format. Supported values: `json`, `text`. |

JSON logs are the default for production and log aggregation systems. Text logs are intended for local development.

Every HTTP response includes `X-Request-ID`. If the request provides a valid bounded `X-Request-ID`, SQLWarden preserves it; otherwise it generates one. Access logs include request ID, route, path, response status, duration, remote IP, user agent, and resolved account/resource identifiers when available.

Server logs include request-aware operational events for authentication, authorization failures, resource mutation, database engine capability lookup, schema inspection, live database sessions, and query cursor lifecycle. `debug` enables lower-level diagnostics such as capability resolution and schema response summaries.

Server logs do not include request bodies, authorization headers, DSNs, SQL text, bind parameters, raw query strings, or row values by default.

## Database

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `db.driver` | `DB_DRIVER` | `--db-driver` | `sqlite` | Application database driver. Supported values: `sqlite`, `postgres`. |
| `db.dsn` | `DB_DSN` | `--db-dsn` | `~/.sqlwarden/sqlwarden.db` | SQLite path or PostgreSQL DSN. `~` is expanded for SQLite. |
| `db.automigrate` | `DB_AUTOMIGRATE` | `--db-automigrate` | `true` | Runs embedded migrations at startup. |
| `db.log_queries` | `DB_LOG_QUERIES` | `--db-log-queries` | `false` | Logs application database SQL text. Use only for short-lived debugging. |

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
| `jwt.access_token_ttl` | `JWT_ACCESS_TOKEN_TTL` | `--jwt-access-token-ttl` | `24h` | Access token lifetime. Examples: `8h`, `30m`. |
| `encryption.key` | `ENCRYPTION_KEY` | `--encryption-key` | Development-only secret | Application encryption key for encrypted values such as DSNs. Replace in every real deployment. |
| `encryption.previous_keys` | `ENCRYPTION_PREVIOUS_KEYS` | `--encryption-previous-keys` | Empty | Comma-separated retired encryption keys retained for decrypting old ciphertext during rotation. |
| `sessions.revocation_enabled` | `SESSIONS_REVOCATION_ENABLED` | `--sessions-revocation-enabled` | `true` | Enables database-backed auth session and org access-session revocation checks. |

Do not use the default secrets outside local development.

Session revocation is enabled by default so administrators can invalidate sessions. Very small single-user deployments can disable it to reduce session-check overhead:

```sh
SESSIONS_REVOCATION_ENABLED=false
```

## Interactive Queries

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `query.max_result_rows` | `QUERY_MAX_RESULT_ROWS` | `--query-max-result-rows` | `10000` | Maximum rows returned by an interactive query result. |
| `query.max_result_bytes` | `QUERY_MAX_RESULT_BYTES` | `--query-max-result-bytes` | `26214400` | Approximate maximum row payload bytes returned by an interactive query result. |

These limits apply to interactive IDE query responses. Future export workflows should use dedicated streaming/export limits instead of relying on interactive query caps.

The same limits apply to HTTP query cursors. Direct `/query` responses are capped once per response. Query-cursor start and fetch responses are capped per page; clients can continue fetching while the response has `exhausted=false`.

For DQL/select-style queries, the IDE can request cursor-backed results through `/query`. When the selected target engine supports cursor-backed results, the first response includes the first page plus cursor metadata. Engines that do not support cursor-backed results fall back to the bounded direct query path. Cursor state is process-local and tied to the authenticated live database session; it is not durable query history.

## Background Jobs

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `jobs.worker_count` | `JOBS_WORKER_COUNT` | `--jobs-worker-count` | `2` | Number of in-process background job workers. |
| `jobs.poll_interval` | `JOBS_POLL_INTERVAL` | `--jobs-poll-interval` | `1s` | How often workers poll for due queued jobs. |
| `jobs.claim_lease` | `JOBS_CLAIM_LEASE` | `--jobs-claim-lease` | `5m` | Lease duration for a claimed running job before another worker may recover it. |
| `jobs.completed_retention` | `JOBS_COMPLETED_RETENTION` | `--jobs-completed-retention` | `168h` | How long succeeded, failed, and cancelled job records are retained. |

Jobs are persisted in the application database. Workers always run inside the API process and use database claim leases so a future separate worker binary can use the same job table safely. Job scheduling is best effort: due jobs run when a worker is available, with higher-priority due jobs claimed before lower-priority due jobs. Internal maintenance such as stale file-content cleanup uses this framework.

User-facing jobs can also persist progress events. Events are read through the scoped job API with an `after_id` marker so clients can poll only for new events. Events follow the parent job retention period configured by `jobs.completed_retention`; there is no separate event retention setting.

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
| `files.revisions.enabled` | `FILES_REVISIONS_ENABLED` | `--files-revisions-enabled` | `true` | Enables saved-file revisions. |
| `files.revisions.keep_latest` | `FILES_REVISIONS_KEEP_LATEST` | `--files-revisions-keep-latest` | `50` | Number of old saved-file revisions retained per file when revisions are enabled. |

The server stores workspace file content on the local filesystem by default. The storage implementation has internal backend plumbing for future expansion, but the server-facing configuration should normally only need the root directory and revision settings.

To disable revisions:

```sh
FILES_REVISIONS_ENABLED=false
```

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

| Config key | Environment | CLI flag | Default | Notes |
| --- | --- | --- | --- | --- |
| `notifications.email` | `NOTIFICATIONS_EMAIL` | `--notifications-email` | Empty | Email address that receives error notifications. |
| `smtp.host` | `SMTP_HOST` | `--smtp-host` | `example.smtp.host` | SMTP server host. |
| `smtp.port` | `SMTP_PORT` | `--smtp-port` | `25` | SMTP server port. |
| `smtp.username` | `SMTP_USERNAME` | `--smtp-username` | `example_username` | SMTP username. |
| `smtp.password` | `SMTP_PASSWORD` | `--smtp-password` | `pa55word` | SMTP password. |
| `smtp.from` | `SMTP_FROM` | `--smtp-from` | `Example Name <no_reply@example.org>` | Default SMTP sender. |

Email is optional today. Configure it when error notification delivery is needed.

## Production Checklist

- Set `BASE_URL` to the public URL users will access.
- Replace `COOKIE_SECRET_KEY`, `JWT_SECRET_KEY`, and `ENCRYPTION_KEY`.
- Decide whether to use SQLite or PostgreSQL for the application database.
- Persist `~/.sqlwarden` or explicitly configure database and file storage paths.
- Keep `LOG_FORMAT=json` for production log collection.
- Disable `DB_LOG_QUERIES` unless actively debugging because it can log SQL text.
- Decide whether `PERSONAL_SPACES_ENABLED` should be enabled.
- Leave `DRIVERS_SQLITE_ALLOWED_SOURCES` empty unless local SQLite target access is intentional.
- Use HTTPS through a reverse proxy or SQLWarden built-in TLS.
- Review `SESSIONS_REVOCATION_ENABLED` for the deployment size and account lifecycle needs.
