# SQLWarden Helm chart (experimental)

This chart is experimental. It is not built or linted in CI, it is not
published to any chart repository, and its values may change without notice.
Install it from a checkout of this repository.

## Modes

| `mode` | Deployments | Database |
| --- | --- | --- |
| `all` (default) | One `all` Deployment; the process serves the API and runs jobs | PostgreSQL or SQLite |
| `split` | `api` Deployment (HTTP API and UI) plus `jobs` Deployment (job runner, scheduler, settings watcher) | PostgreSQL only |

Every Deployment runs the same image and binary and differs only in
`PROCESS_KINDS`. Replica counts are separate values: `all.replicaCount`,
`api.replicaCount`, and `jobs.replicaCount`.

A `jobs` pod serves only `/livez` and `/readyz`; it has no API routes. In split
mode, session-revoke retry jobs run in the `jobs` pods, which hold no live
target-database sessions. They cannot close sessions held by an `api` pod until
remote execution lands (SQLW-164).

SQLite is accepted only in `mode: all` with `all.replicaCount: 1` and
`migration.enabled: false`; `values.schema.json` rejects other combinations and
the templates fail rendering with the same reason. The process then migrates
itself on start. Point `database.dsn` at a file on the storage volume.

## Install

```sh
helm install sqlwarden deploy/experimental/helm/sqlwarden \
  --namespace sqlwarden --create-namespace \
  --set database.dsn='postgres://sqlwarden:...@postgres:5432/sqlwarden?sslmode=require' \
  --set secrets.cookieSecretKey="$(openssl rand -hex 32)" \
  --set secrets.jwtSecretKey="$(openssl rand -hex 32)" \
  --set secrets.encryptionKey="$(openssl rand -hex 32)"
```

Add `--set mode=split --set storage.existingClaim=<rwx-claim>` for the split
topology. The chart does not deploy PostgreSQL.

## Required secrets

`cookie_secret_key`, `jwt_secret_key`, `encryption_key`, and the database DSN
(`db_dsn` by default) are required. They are rendered into a Secret from
`secrets.*` and `database.dsn`, or read from Secrets you manage:
`secrets.existingSecret` (keys `cookie_secret_key`, `jwt_secret_key`,
`encryption_key`) and `database.existingSecret` (key `database.dsnKey`).
Secrets are injected through `secretKeyRef` and are never written to the
ConfigMap. Losing `encryption_key` makes stored connection credentials
unreadable.

## Migrations

`migration.enabled` renders a Job annotated
`helm.sh/hook: pre-install,pre-upgrade` that runs `sqlwarden migrate`, so it
completes before new serving replicas start and Helm fails the release if it
does not. The command holds a database-level migration lock, so a retry or a
concurrent release waits. Serving Deployments set `DB_AUTOMIGRATE=false`; the
application rejects automigrate when `api` or `jobs` is selected explicitly.
Only the `all` Deployment migrates itself, and only when `migration.enabled` is
`false`.

## Probes

Both probes hit the container's HTTP port: liveness is `GET /livez` and
readiness is `GET /readyz`. A startup probe also uses `/readyz`.

## Storage

Workspace file content lives under `storage.mountPath/files`. With no
`storage.existingClaim` the chart mounts an emptyDir, which does not survive a
restart and is single-pod only. Split mode and any mode with more than one
serving replica require a ReadWriteMany claim; the chart refuses to render
otherwise.

## Shutdown

SIGTERM starts a graceful shutdown bounded by `config.shutdownTimeout`. Keep
`config.terminationGracePeriodSeconds` above it.

## Security

Pods run as non-root (uid 1000) with `RuntimeDefault` seccomp, a read-only root
filesystem, no privilege escalation, and all capabilities dropped. Service
account tokens are not mounted; SQLWarden never calls the Kubernetes API.

## Not included

HorizontalPodAutoscaler, PodDisruptionBudget, NetworkPolicy, Ingress,
ServiceMonitor, and image digest pinning.
