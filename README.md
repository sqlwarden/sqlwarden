# SQLWarden

SQLWarden is a self-hosted database access platform and SQL editor for teams that need controlled access to production and non-production databases.

It combines a Go backend, embedded React application, custom RBAC, workspace-scoped database resources, query execution, file storage, and an editor built around organizations, workspaces, environments, and connections.

SQLWarden is pre-1.0. The API and database schema may change while the project is still being shaped.

## Features

- Self-hosted server with an embedded web UI.
- Organization, workspace, environment, connection, user, team, role, and policy management.
- Custom additive RBAC with effective permissions APIs for frontend capability checks.
- Workspace and personal-space models for team and individual database work.
- Target database engines for PostgreSQL, CockroachDB, Neon, Supabase, YugabyteDB, MySQL, MariaDB, TiDB, Oracle, SQL Server, and gated SQLite.
- Query execution with foreground cancellation.
- Single-process deployment by default, or a split API/connector topology (with a Helm chart) that keeps live target sessions and stored credential resolution in the connector.
- SQL editor with workspace tabs, explorer, editor tabs, console tabs, result panes, and same-browser multi-window sync.
- Workspace file APIs with private and shared file scopes.
- SQLite application database by default, with PostgreSQL application database support.
- Configuration through file, environment variables, mounted secret files, and CLI flags.
- Release automation through Release Please and GoReleaser.

## Quick Start

Prerequisites for local builds:

- Go 1.26.4 or newer.
- Bun for frontend dependency installation and builds.

The default configuration stores the application database at `~/.sqlwarden/sqlwarden.db` and file content at `~/.sqlwarden/files`.

```sh
make build
./dist/sqlwarden
```

Open `http://localhost:6020` and complete first-run setup.

For development with API query logging enabled:

```sh
make run
```

## Docker

Released images are published to GitHub Container Registry.

```sh
docker run -d \
  --name sqlwarden \
  -p 6020:6020 \
  -v sqlwarden-data:/var/lib/sqlwarden \
  -e BASE_URL=http://localhost:6020 \
  -e DB_DSN=/var/lib/sqlwarden/sqlwarden.db \
  -e FILES_ROOT_DIR=/var/lib/sqlwarden/files \
  -e COOKIE_SECRET_KEY=replace-with-a-random-secret \
  -e JWT_SECRET_KEY=replace-with-a-random-secret \
  -e ENCRYPTION_KEY=replace-with-a-random-secret \
  ghcr.io/sqlwarden/sqlwarden:latest
```

Use the latest published release tag for production deployments. The secrets above are placeholders and must be replaced before exposing the service to users.

## Configuration

SQLWarden can be configured with a config file, environment variables, mounted secret files, or CLI flags. See [docs/configuration.md](docs/configuration.md) for the full reference.

Common settings:

```sh
BASE_URL=http://localhost:6020
HTTP_PORT=6020
DB_DRIVER=sqlite
DB_DSN=~/.sqlwarden/sqlwarden.db
FILES_ROOT_DIR=~/.sqlwarden/files
```

To see available flags:

```sh
./dist/sqlwarden --help
```

## Development

Install frontend dependencies:

```sh
make frontend/install
```

Build the frontend and backend:

```sh
make build
```

Run backend tests:

```sh
make test
```

Run the full local audit suite:

```sh
make audit
```

Run the Vite development server with API proxying:

```sh
make frontend/dev
```

Install repository-managed git hooks:

```sh
make hooks/install
```

## Repository Layout

```text
assets/                       Embedded core migrations, email templates, and frontend build output
cmd/api/                      Thin server entrypoint (serve, migrate, rotate-keys)
deploy/helm/                  Split API/connector Helm chart and its validation tests
docs/                         Architecture and operator documentation
ee/                           Enterprise Edition composition tree; never imported by core
frontend/                     React application
internal/config/              Bootstrap configuration loading, validation, diagnostics
internal/app/                 Service-graph composition and process lifecycle
internal/community/           Community Edition composition root
internal/edition/             Edition seam between core and edition modules
internal/web/                 HTTP transport and all/api/connector process adapters
internal/settings/            Runtime settings service
internal/identity/            Account identity and authentication service
internal/access/              RBAC permissions, roles, policies, and enforcer
internal/audit/               Durable audit event contracts and core writer
internal/catalog/             Organization, workspace, environment, connection service
internal/execution/           Target execution boundary (local and connector runtimes)
internal/credentials/         Local-execution stored credential resolution
internal/connection/          Live target sessions beneath the local runtime
internal/engine/              Target database engines and capability interfaces
internal/database/            Bun models and application database setup
internal/files/               Workspace file service
internal/filestore/           File content storage backend
internal/jobs/                Durable background job framework
internal/architecture/        Executable repository structure and boundary rules
pkg/result/                   Normalized target query result types
```

`cmd/api` is intentionally thin: it loads configuration, builds the Community edition through `internal/app`, and runs the selected process kinds. New entrypoints, including desktop packaging, should compose through `internal/app` and reuse `internal/web`.

## API And Architecture

The committed architecture reference is [docs/sqlwarden-architecture.md](docs/sqlwarden-architecture.md). Kubernetes deployment of the split topology is described in [docs/kubernetes.md](docs/kubernetes.md).

One binary serves the process kinds selected by `process_kinds`:

- `all` (default): public HTTP API, embedded UI, background workers, and in-process target execution.
- `api`: public HTTP API, embedded UI, and background workers; target execution is forwarded to a connector over an authenticated internal protocol.
- `connector`: internal execution endpoint that owns live target sessions and resolves stored target credentials.

A split deployment currently supports exactly one connector replica.

The API currently uses `/api/v1`, standard JSON error envelopes, and paginated list envelopes for UI-facing list endpoints. SQLWarden is still before v1, so compatibility-breaking cleanup can happen before the first stable release.

## Security Notes

SQLWarden is designed for self-hosted deployments. Operators are responsible for network placement, TLS termination or built-in TLS configuration, secret management, backups, and database access boundaries.

Before production use:

- Replace `COOKIE_SECRET_KEY`, `JWT_SECRET_KEY`, and `ENCRYPTION_KEY`.
- Use HTTPS through a reverse proxy or built-in TLS.
- Review whether personal spaces should be enabled.
- Review whether target SQLite connections should be enabled.
- Use PostgreSQL for the application database when SQLite is not appropriate for the deployment size or operating model.

Security-sensitive defaults are intended to make local development easy, not to harden production automatically.

## Releases

The project uses conventional commits, Release Please, and GoReleaser. Stable releases and manually initiated release candidates publish server binaries and container images from version tags. Release candidates use version-specific `-rc.N` tags and never update the `latest` container tag.

Use squash or rebase workflows that keep `main` linear and preserve clear conventional commit messages for user-facing changes.
