# Experimental Helm chart

`sqlwarden/` is an experimental Helm chart. It has no CI job and is not
published as a chart; install it from a checkout. It runs either one
all-in-one Deployment (`mode: all`) or separate `api` and `jobs` Deployments
(`mode: split`, PostgreSQL only). See [sqlwarden/README.md](sqlwarden/README.md)
for modes, required secrets, migrations, and storage.

`go test ./deploy/...` checks the chart against the application's
configuration keys without needing a `helm` binary.
