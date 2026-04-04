# API Contract

## Bootstrap

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

## Connections List

`GET /api/v1/orgs/{org_slug}/workspaces/{ws_id}/connections` returns:

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
- `environment_id`
- `driver`
- `access_mode`

## Workspace Policy List

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

## Org Members And Teams

- `GET /api/v1/orgs/{org_slug}/members` supports `q`, `sort`, and `order`
- `GET /api/v1/orgs/{org_slug}/teams` supports `q`, `sort`, and `order`

## Workspaces And Environments

- `GET /api/v1/orgs/{org_slug}/workspaces` supports `q`, `sort`, and `order`
- `GET /api/v1/orgs/{org_slug}/workspaces/{ws_id}/environments` supports `sort` and `order`

## Mutability

- workspaces: mutable `name`, `description`; immutable `org_id`, `owner_type`, `owner_id`
- environments: mutable `name`, `description`; immutable `workspace_id`, `org_id`, `owner_type`, `owner_id`
- teams: mutable `name`; immutable `slug`, `org_id`
- connections: mutable `name`, `dsn`, `access_mode`; immutable `workspace_id`, `environment_id`, `driver`

## Explicitly Deferred Items

- organization rename is deferred from v1 and returns `405 Method Not Allowed`
- organization delete is deferred from v1 and returns `405 Method Not Allowed`
