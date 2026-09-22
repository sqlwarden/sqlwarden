# Kubernetes Deployment

The chart in `deploy/helm/sqlwarden` is the supported way to run SQLWarden on Kubernetes. Its README documents values, sizing assumptions, and storage; this page covers the deployment model.

## Topology

One image runs as two Deployments that differ only in `PROCESS_KINDS`:

- `api` serves the public HTTP API and the embedded UI. It holds request state and cached schema metadata, and scales out with replicas.
- `connector` holds the live sessions to target databases. It is fixed at one replica because the static session directory resolves a single connector address; a second replica would own sessions nothing can route to. The application rejects `connector.replicas` above `1` with the static directory, so this is enforced in the process as well as in the chart.

Both processes share `connector.grant_signing_key`, which signs the short-lived execution grants the api process sends to the connector.

## Migrations

A Job annotated `helm.sh/hook: pre-install,pre-upgrade` runs `sqlwarden migrate` before any new serving replica starts, and Helm fails the release if it does not complete. The command takes a migration lock against the application database for the whole run, so a retry or a concurrent rollout waits instead of interleaving. `db.migration_timeout` cancels the wait or signals a running migration to stop without releasing the lock early.

Every serving replica runs with `DB_AUTOMIGRATE=false` and would refuse to start otherwise. See [Configuration](configuration.md) for the rule.

## Health and shutdown

Probes use `GET /healthz` for liveness and `GET /readyz` for startup and readiness. The connector serves them on a separate credential-free listener (`connector.health_address`, default `6022`) because its execution listener requires transport credentials; do not expose that port outside the cluster.

`SIGTERM` starts a graceful shutdown bounded by `shutdown_timeout`, draining resources in reverse startup order. Keep the pod's termination grace period above `shutdown_timeout`.

## Identity

Each process kind has its own ServiceAccount with no mounted token. SQLWarden never calls the Kubernetes API; the optional Roles have empty rule sets, so even a token injected by cluster policy has no permissions.
