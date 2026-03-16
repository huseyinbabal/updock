# Audit Log

Updock maintains a persistent, append-only audit log of every action it
performs. This provides a complete trail for debugging, compliance, and
understanding what changed and when.

## Configuration

| Argument | Environment Variable | Default |
|----------|---------------------|---------|
| `--audit-log` | `UPDOCK_AUDIT_LOG` | `/var/lib/updock/audit.json` |

Mount a volume to persist the audit log across container restarts:

```yaml
services:
  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - updock-data:/var/lib/updock
      - /var/run/docker.sock:/var/run/docker.sock
volumes:
  updock-data:
```

## Event Types

Every audit entry has a `type` field indicating what happened:

| Type | Description |
|------|-------------|
| `update.started` | An update check found a newer image available. |
| `update.pulled` | A new image was pulled from the registry. |
| `update.applied` | A container was recreated with the new image. |
| `update.skipped` | Update was skipped (policy, window, or approval). |
| `update.failed` | Update attempt failed (pull error, start error). |
| `rollback.auto` | Automatic rollback triggered by health check failure. |
| `rollback.manual` | Manual rollback triggered via API or Web UI. |
| `approval.pending` | Update queued, waiting for manual approval. |
| `approval.granted` | Manual approval granted via API or Web UI. |
| `approval.denied` | Manual approval denied. |

## Entry Format

Each entry contains:

```json
{
  "id": 42,
  "timestamp": "2026-03-16T02:15:00Z",
  "type": "update.applied",
  "container_name": "nginx",
  "container_id": "abc123def456",
  "image": "nginx:1.25",
  "old_image_id": "sha256:aaa...",
  "new_image_id": "sha256:bbb...",
  "policy": "conservative",
  "actor": "system",
  "message": "Updated nginx from 1.25.3 to 1.25.4"
}
```

| Field | Description |
|-------|-------------|
| `id` | Monotonically increasing entry ID. |
| `timestamp` | ISO 8601 timestamp. |
| `type` | Event type (see table above). |
| `actor` | Who initiated the action: `system`, `api`, `ui`, `schedule`. |
| `policy` | Policy name that applied to this container. |
| `message` | Human-readable description. |

## API Access

Query the audit log via the REST API:

```bash
# Get last 100 entries
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/audit

# Filter by container
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/audit?container=nginx&limit=50"
```

See [HTTP API](http-api.md) for full endpoint documentation.

## Retention

The audit log retains up to **10,000 entries** in memory and persists them
to the configured file path. Older entries are evicted on a FIFO basis.

!!! tip
    For long-term retention, forward audit entries to an external system
    using webhook notifications with a custom template.
