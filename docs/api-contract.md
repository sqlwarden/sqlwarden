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
