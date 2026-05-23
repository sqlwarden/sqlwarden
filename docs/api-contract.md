# API Contract

## Bootstrap

### `POST /api/setup`

Creates the first account and instance admin. In `ACCESS_MODE=single_user`, setup also seeds a local organization:

```json
{
  "account": {},
  "access_token": "...",
  "organization": {
    "slug": "local",
    "name": "Local"
  }
}
```

In `ACCESS_MODE=multi_user`, the response omits `organization`.

### `GET /api/setup/status`

Response:

```json
{
  "configured": true
}
```

### `GET /api/v1/session`

Response fields:

- `account`
- `organizations`
- `is_instance_admin`
- `personal_spaces_enabled`
- `feature_flags`

## Error Semantics

- `400`: malformed JSON or primitive parse errors
- `401`: unauthenticated
- `403`: authenticated but forbidden
- `404`: resource missing or not in the requested scope
- `409`: state conflict
- `422`: field validation and duplicate-name errors

## Shared List Query Parameters

- `page`
- `page_size`
- `sort`
- `order`
- `q`

## List Endpoints

### Connections

Canonical route:

`GET /api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments/{env_id}/connections` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Supported query parameters:

- shared list query parameters
- `driver`
- `access_mode`

Notes:

- every connection must belong to an environment
- every workspace auto-creates a `Default` environment
- workspace-level connection routes are still accepted as compatibility aliases, but the environment-nested route is the primary contract

### Workspace Policies

`GET /api/v1/orgs/{org_slug}/workspaces/{ws_id}/policies` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Each item includes UI-renderable subject and resource metadata:

- `subject_id`
- `subject_type`
- `subject_name`
- `resource_id`
- `resource_type`
- `resource_name`
- `permission`
- `role_id`
- `role_name`

### Org Members

`GET /api/v1/orgs/{org_slug}/members` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Supported query parameters:

- shared list query parameters
- `role`

### Teams

`GET /api/v1/orgs/{org_slug}/teams` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Supported query parameters:

- shared list query parameters
- `slug`

### Workspaces

`GET /api/v1/orgs/{org_slug}/workspaces` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Supported query parameters:

- shared list query parameters
- `name`

### Environments

`GET /api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments` returns:

```json
{
  "items": [],
  "page": 1,
  "page_size": 25,
  "total": 0
}
```

Supported query parameters:

- shared list query parameters
- `name`

### Personal Spaces

- personal-space routes are under `/api/v1/me`
- `GET /api/v1/me` is an account/self route
- `/api/v1/me/workspaces...` routes require authentication and the personal-spaces feature gate
- personal-space resources are owner-filtered by `owner_type = "space"` and `owner_id = current_account.id`
- personal-space routes do not use org RBAC and never expose org-owned resources
- `GET /api/v1/me/workspaces` returns the same paginated workspace envelope and supports shared list query parameters plus `name`
- `GET /api/v1/me/workspaces/{ws_id}/environments` returns the same paginated environment envelope and supports shared list query parameters plus `name`
- `GET /api/v1/me/workspaces/{ws_id}/environments/{env_id}/connections` returns the same paginated connection envelope and supports shared list query parameters plus `driver` and `access_mode`
- `GET /api/v1/me/workspaces/{ws_id}/connections` is accepted as a workspace-level compatibility alias and supports the same connection query parameters

Single-user/local mode should use the seeded `local` organization and org routes for normal managed resources. Personal spaces remain optional user-owned sandboxes, not the single-user authorization model.

## Search / Filter / Sort / Pagination

- org members: pagination, search, role filter, and sort
- teams: pagination, search, slug filter, and sort
- workspaces: pagination, search, name filter, and sort
- environments: pagination, search, name filter, and sort
- connections: pagination, search, driver filter, access-mode filter, and sort
- workspace policies: pagination, search, subject/resource filtering, permission filtering, and sort
- personal-space workspaces: pagination, search, name filter, and sort
- personal-space environments: pagination, search, name filter, and sort
- personal-space connections: pagination, search, driver filter, access-mode filter, and sort

## Future Resources

- Future UI-facing list resources should use the same paginated response envelope.
- Future list endpoints should accept the shared query parameters and add only resource-specific flat filters when needed.
- Clients should be able to assume `items` is always an array and never `null`.
- Future workspace child resources should preserve a strict parent chain in `resource_hierarchy`.

## Mutability

- workspaces: mutable `name`, `description`; immutable `org_id`, `owner_type`, `owner_id`
- environments: mutable `name`, `description`; immutable `workspace_id`
- teams: mutable `name`; immutable `slug`, `org_id`
- connections: mutable `name`, `dsn`, `access_mode`; immutable `workspace_id`, `environment_id`, `driver`

## Explicitly Deferred Items

- organization rename is deferred from v1 and returns `405 Method Not Allowed`
- organization delete is deferred from v1 and returns `405 Method Not Allowed`
