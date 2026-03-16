# Web UI

Updock includes a built-in web dashboard for monitoring container status, viewing update history, and triggering manual updates.

## Accessing the Dashboard

The Web UI is served at the configured HTTP address (default `:8080`):

```
http://localhost:8080
```

!!! info
    The Web UI is enabled by default. Disable it with `--http-enabled=false` if you only need the CLI.

## Features

### Container List

The main dashboard displays all monitored containers in a table with:

- **Container name** and ID
- **Current image** tag
- **Status** — running, stopped, or restarting
- **Update available** — indicator showing whether a newer image exists
- **Latest image** — the newest available tag from the registry
- **Last checked** — timestamp of the most recent update check

Containers with available updates are highlighted for quick identification.

### Update History

The history view shows a chronological log of all updates performed:

| Column | Description |
|---|---|
| Container | Name of the updated container |
| Old Image | Previous image reference |
| New Image | New image reference |
| Updated At | Timestamp of the update |
| Dry Run | Whether the update was a dry-run |

History is persisted in memory and available as long as the Updock instance is running.

### Manual Update Trigger

A button on the dashboard allows triggering an immediate update cycle. You can either:

- **Update all** monitored containers
- **Select specific** containers to update

!!! note
    Manual triggers use the same logic as scheduled updates — including label filtering, dependency ordering, and lifecycle hooks.

### Health Status

The dashboard header displays:

- **Updock version** and uptime
- **Connection status** to the Docker daemon
- **Number of monitored containers**
- **Next scheduled check** countdown
- **Scope** (if configured)

### Audit Log

The audit log view provides a complete trail of every action Updock has
performed:

| Column | Description |
|---|---|
| Timestamp | When the action occurred |
| Type | Event type (update.applied, update.skipped, rollback.auto, etc.) |
| Container | Container name |
| Actor | Who initiated: system, api, ui, schedule |
| Policy | Which policy was applied |
| Message | Human-readable description |

See [Audit Log](audit-log.md) for event types and API access.

### Pending Approvals

When containers use `approve: manual` in their policy, pending updates appear
in a dedicated section. Each pending update shows the container name, current
image, available image, and approve/deny buttons.

### Auto-Refresh

The dashboard auto-refreshes every **30 seconds** by default. During an active update cycle, the refresh interval drops to **5 seconds** for real-time progress tracking.

## Authentication

When `--http-api-token` is set, the Web UI prompts for the token on first access. The token is stored in the browser's local storage for subsequent visits.

## Configuration

```yaml
services:
  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    ports:
      - "8080:8080"
    environment:
      - UPDOCK_HTTP_ENABLED=true
      - UPDOCK_HTTP_ADDR=:8080
      - UPDOCK_HTTP_API_TOKEN=my-secret-token
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

!!! warning "Security"
    If exposing the Web UI to the internet, always set an API token and consider placing a reverse proxy with TLS in front of Updock.

## Customization

The Web UI uses the same HTTP address as the [REST API](http-api.md). All data displayed in the UI is fetched from the API endpoints, so any API-level filtering (scope, labels) is reflected in the dashboard.
