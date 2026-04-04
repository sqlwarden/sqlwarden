# API UI Readiness

## Current State

- bootstrap contract frozen for setup and authenticated session bootstrap
- normalized UI-facing error semantics documented in `docs/api-contract.md`
- unified paginated list contract implemented for org members, teams, workspaces, environments, connections, and workspace policies
- mutability rules enforced for workspace, environment, team, and connection updates
- organization rename/delete explicitly deferred from the v1 API contract
- workspace policy payloads include renderable subject and resource metadata
- future UI-facing list resources are expected to use the same `items/page/page_size/total` contract with shared query parameters
