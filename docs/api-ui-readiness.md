# API UI Readiness

## Current State

- bootstrap contract frozen for setup and authenticated session bootstrap
- normalized UI-facing error semantics documented in `docs/api-contract.md`
- search, filter, sort, and pagination contract implemented for v1 list screens
- mutability rules enforced for workspace, environment, team, and connection updates
- organization rename/delete explicitly deferred from the v1 API contract
- workspace policy payloads include renderable subject and resource metadata
- remaining UI contract work tracked in `docs/api-contract.md`
