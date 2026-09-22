# SQLWarden Helm chart

This split topology requires PostgreSQL. SQLite remains available for the
single-process `all` deployment, but cannot be shared safely by the API,
connector, and migration Job.

Runs SQLWarden on Kubernetes as two Deployments from one image:

| Process | Role | Replicas |
| --- | --- | --- |
| `api` | Public HTTP API and embedded UI | 1 by default; more requires a shared file volume |
| `connector` | Live sessions to target databases | Fixed at 1 |

Both containers run the same binary and differ only in `PROCESS_KINDS`. A
pre-install/pre-upgrade Job runs `sqlwarden migrate`; serving replicas never
migrate.

## Install

```sh
helm install sqlwarden deploy/helm/sqlwarden \
  --namespace sqlwarden --create-namespace \
  --set database.dsn='postgres://sqlwarden:...@postgres:5432/sqlwarden?sslmode=require' \
  --set secrets.cookieSecretKey="$(openssl rand -hex 32)" \
  --set secrets.jwtSecretKey="$(openssl rand -hex 32)" \
  --set secrets.encryptionKey="$(openssl rand -hex 32)" \
  --set secrets.connectorGrantSigningKey="$(openssl rand -hex 32)"
```

To keep secrets out of values, create the Secret yourself and set
`secrets.existingSecret` (keys `cookie_secret_key`, `jwt_secret_key`,
`encryption_key`, `connector_grant_signing_key`) and/or
`database.existingSecret` with key `db_dsn`.

The chart does not deploy PostgreSQL. Point `database.dsn` at a database you
operate.

## Migrations

`migration.enabled` renders a Job annotated
`helm.sh/hook: pre-install,pre-upgrade`, so it completes before any new serving
replica starts, and Helm fails the release if it does not. The Job runs
`sqlwarden migrate`, which holds a database-level migration lock for the whole
run: a retry, a concurrent release, or a second cluster pointed at the same
database waits instead of interleaving. `database.migrationTimeout` cancels
lock acquisition or signals a running migration to stop; the lock remains held
until the runner returns. `migration.activeDeadlineSeconds` should stay above
it so the process can report the timeout itself.

Every serving replica sets `DB_AUTOMIGRATE=false`, and the application rejects
`db.automigrate=true` whenever `process_kinds` explicitly selects `api` or
`connector`. Those processes are one of many identical replicas, so migrating
from them races every other replica.

Set `migration.enabled=false` only when migrations are applied outside the
release; the serving replicas will not apply them.

## Connector single-replica constraint

`connector.replicas` is constrained to `1` by `values.schema.json`, by a render
guard, and by the application's own configuration validation. The static session
directory resolves a single connector address, so a second replica would own
live sessions nothing can route to. The connector Deployment uses the `Recreate`
strategy for the same reason. Multi-replica connectors need a shared session
directory, which is not part of v1.

## Probes

| Probe | api | connector |
| --- | --- | --- |
| startup | `GET /readyz` on the HTTP port | `GET /readyz` on the health port |
| liveness | `GET /healthz` | `GET /healthz` |
| readiness | `GET /readyz` | `GET /readyz` |

`/healthz` reports whether the process is built and not shutting down — only a
restart fixes a failure. `/readyz` reports whether this replica can serve right
now, including its dependencies. The connector's execution listener requires
transport credentials, so the connector serves probes on a separate
credential-free listener (`connector.healthPort`, default 6022) that should not
be exposed outside the cluster.

## Shutdown

SIGTERM starts a graceful shutdown bounded by `config.shutdownTimeout`;
in-flight work drains in reverse startup order. Keep
`config.terminationGracePeriodSeconds` above `config.shutdownTimeout` so the
kubelet does not SIGKILL a draining replica.

## Sizing

Defaults suit a small production install. They are starting points; measure
before changing.

| Process | Requests | Limits | Assumption |
| --- | --- | --- | --- |
| api | 250m / 512Mi | 1 / 1Gi | Request-bound. Result rows stream through the connector, so a replica holds request state, cached schema metadata, and the embedded UI. Roughly 50 concurrent interactive users per replica; scale out, not up. |
| connector | 500m / 1Gi | 2 / 2Gi | Session-bound. Holds every live target-database session and buffers result pages, so memory tracks concurrent sessions and page size. Roughly 100 live sessions at default page sizes; raise memory before CPU. |
| migrate | 100m / 256Mi | 1 / 512Mi | Runs DDL and waits; cost is in the database, not the Job. |

## Storage

Workspace file content uses a filesystem backend under
`api.files.mountPath`. With no `api.files.existingClaim` the chart mounts an
emptyDir, which does not survive a pod restart and is single-replica only. Set
`api.files.existingClaim` to a PersistentVolumeClaim — ReadWriteMany if
`api.replicas` is above 1; the chart refuses to render otherwise.

## Security

Pods run as non-root (uid 1000) with `RuntimeDefault` seccomp, a read-only root
filesystem, no privilege escalation, and all capabilities dropped. Each process
kind has its own ServiceAccount with `automountServiceAccountToken: false`;
SQLWarden never calls the Kubernetes API. The Roles under `rbac.create` have
empty rule sets, making the zero-permission boundary explicit for every process
kind even if cluster policy injects a token.

## Not in v1

HorizontalPodAutoscaler, PodDisruptionBudget, NetworkPolicy,
ServiceMonitor/tracing, Ingress, and image digest pinning.
