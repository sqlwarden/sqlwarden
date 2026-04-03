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
