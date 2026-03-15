# HTTP API

Updock includes a built-in REST API for monitoring containers, triggering updates, and viewing history. The API is served alongside the [Web UI](web-ui.md) on the configured HTTP address.

## Configuration

| Argument | Environment Variable | Default |
|---|---|---|
| `--http-enabled` | `UPDOCK_HTTP_ENABLED` | `true` |
| `--http-addr` | `UPDOCK_HTTP_ADDR` | `:8080` |
| `--http-api-token` | `UPDOCK_HTTP_API_TOKEN` | *none* |

## Authentication

When `--http-api-token` is set, all API requests must include a Bearer token in the `Authorization` header:

```bash
curl -H "Authorization: Bearer my-secret-token" http://localhost:8080/api/health
```

!!! warning
    Without a token set, the API is **unauthenticated**. Always configure a token when exposing Updock to a network.

## Endpoints

### `GET /api/health`

Returns the health status of the Updock instance.

```bash
curl http://localhost:8080/api/health
```

```json
{
  "status": "healthy",
  "uptime": "48h12m",
  "version": "1.5.0"
}
```

---

### `GET /api/info`

Returns runtime configuration and instance metadata.

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/info
```

```json
{
  "version": "1.5.0",
  "hostname": "server-01",
  "scope": "production",
  "interval": 86400,
  "containers_monitored": 12,
  "dry_run": false
}
```

---

### `GET /api/containers`

Lists all monitored containers and their current update status.

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/containers
```

```json
[
  {
    "id": "abc123",
    "name": "nginx",
    "image": "nginx:1.25",
    "status": "running",
    "update_available": true,
    "latest_image": "nginx:1.26",
    "last_checked": "2025-01-15T10:00:00Z"
  }
]
```

---

### `GET /api/containers/{id}`

Returns details for a specific container by ID or name.

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/containers/nginx
```

```json
{
  "id": "abc123def456",
  "name": "nginx",
  "image": "nginx:1.25",
  "status": "running",
  "update_available": true,
  "latest_image": "nginx:1.26",
  "labels": {
    "com.updock.enable": "true"
  },
  "last_checked": "2025-01-15T10:00:00Z",
  "last_updated": "2025-01-14T08:00:00Z"
}
```

---

### `POST /api/update`

Triggers an immediate update check cycle. Optionally specify container names in the request body.

```bash
# Update all monitored containers
curl -X POST -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/update
```

```bash
# Update specific containers
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"containers": ["nginx", "redis"]}' \
  http://localhost:8080/api/update
```

```json
{
  "status": "started",
  "containers": ["nginx", "redis"]
}
```

!!! note
    The endpoint returns immediately. The update runs asynchronously. Monitor progress via the Web UI or `/api/containers`.

---

### `GET /api/history`

Returns the update history log.

```bash
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/history?limit=10"
```

```json
[
  {
    "container": "nginx",
    "old_image": "nginx:1.24",
    "new_image": "nginx:1.25",
    "updated_at": "2025-01-15T10:30:00Z",
    "dry_run": false
  }
]
```

| Query Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | `int` | `50` | Maximum number of entries to return |
| `offset` | `int` | `0` | Number of entries to skip |

---

### `GET /api/audit`

Returns the audit log entries. Supports filtering by container name and limiting results.

```bash
# Get last 100 entries
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/audit

# Filter by container
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/audit?container=nginx&limit=50"
```

```json
[
  {
    "id": 42,
    "timestamp": "2026-03-16T02:15:00Z",
    "type": "update.applied",
    "container_name": "nginx",
    "container_id": "abc123def456",
    "image": "nginx:1.25",
    "policy": "conservative",
    "actor": "system",
    "message": "Updated nginx from 1.25.3 to 1.25.4"
  }
]
```

| Query Parameter | Type | Default | Description |
|---|---|---|---|
| `container` | `string` | *none* | Filter by container name |
| `limit` | `int` | `100` | Maximum entries to return |

See [Audit Log](audit-log.md) for event types and entry format.

---

### `GET /metrics`

Prometheus-compatible metrics endpoint. Only available when `--metrics` is enabled. See [Metrics](metrics.md) for details.

```bash
curl http://localhost:8080/metrics
```

!!! info
    The `/metrics` endpoint does **not** require Bearer token authentication to allow Prometheus scraping without custom headers.
