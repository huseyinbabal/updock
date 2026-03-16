# Web Dashboard Usage

Updock ships with a built-in web dashboard at `http://localhost:8080`.
No extra setup required - it is enabled by default.

## Accessing the Dashboard

```yaml
services:
  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"
    environment:
      - UPDOCK_HTTP_API_TOKEN=my-secret-token  # optional
```

Open `http://localhost:8080` in your browser.

## Dashboard Tabs

### Containers

The default view shows all monitored containers with:

- **Name** and **Image** reference
- **Policy** assigned to each container (from `updock.yml`)
- **Status** with color-coded indicators (green=running, red=exited, yellow=paused)
- **Container ID** (short form)

### Audit Log

A chronological list of every action Updock has performed:

- **update.applied** - Container was recreated with a new image
- **update.skipped** - Update was skipped (maintenance window, policy, approval)
- **approval.pending** - Update queued for manual approval
- **rollback.auto** - Automatic rollback triggered

Each entry shows the timestamp, event type, container name, actor
(system/api/schedule), and a human-readable message.

### Policies

View your loaded `updock.yml` configuration:

- **Defined Policies** - All named policies with strategy, approval mode,
  rollback setting, and health timeout
- **Container Assignments** - Per-container policy overrides, maintenance
  windows, and ignore flags
- **Groups** - Container groups with members, strategy, and restart order

### Update History

Shows every update check result with container name, image, whether the
update was applied, any errors, and timestamp.

## Manual Update Trigger

Click **Check for Updates** in the header to run an immediate update
check. The button shows a spinner while checking and displays a toast
notification with results.

## Securing the Dashboard

Set an API token to protect the dashboard:

```yaml
environment:
  - UPDOCK_HTTP_API_TOKEN=my-secret-token
```

All API endpoints (except `/api/health`) require the token via:

- Header: `Authorization: Bearer my-secret-token`
- Query: `?token=my-secret-token`

!!! tip
    Use Docker secrets for the token in production:
    `UPDOCK_HTTP_API_TOKEN=/run/secrets/api_token`

## Disabling the Dashboard

```bash
updock --http-enabled=false
```

This disables both the Web UI and the REST API. Prometheus metrics
are still available at `/metrics` if `--metrics` is enabled.
