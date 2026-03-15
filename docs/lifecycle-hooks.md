# Lifecycle Hooks

Updock supports lifecycle hooks that execute commands at specific points during the update process. Hooks are defined per-container using labels.

!!! warning "Opt-in Required"
    Lifecycle hooks are disabled by default. You must pass `--lifecycle-hooks` (or `UPDOCK_LIFECYCLE_HOOKS=true`) to enable them globally.

## Hook Types

There are four hook stages, executed in order:

| Hook | Label | When It Runs |
|---|---|---|
| **pre-check** | `com.updock.lifecycle.pre-check` | Before checking for an image update |
| **pre-update** | `com.updock.lifecycle.pre-update` | After a new image is found, before stopping the container |
| **post-update** | `com.updock.lifecycle.post-update` | After the container has been recreated and started |
| **post-check** | `com.updock.lifecycle.post-check` | After the update check cycle completes (regardless of update) |

## Defining Hooks

Hooks are set as container labels. The value is the command to execute **inside** the container:

```yaml
services:
  webapp:
    image: myapp:latest
    labels:
      - "com.updock.lifecycle.pre-update=/app/scripts/graceful-shutdown.sh"
      - "com.updock.lifecycle.post-update=/app/scripts/warmup.sh"
```

!!! note
    Commands run via `docker exec` inside the target container. The container must have the referenced binary or script available.

## Timeouts

Each hook has a default timeout of **60 seconds**. You can override this per-hook:

```yaml
labels:
  - "com.updock.lifecycle.pre-update=/app/drain-connections.sh"
  - "com.updock.lifecycle.pre-update.timeout=120"
```

The timeout value is in seconds. If a hook exceeds its timeout, the process is killed and treated as a failure.

## Failure Behavior

Hook failure behavior depends on the stage:

| Hook | On Failure |
|---|---|
| `pre-check` | Update check is **skipped** for this container |
| `pre-update` | Update is **aborted** — container is not stopped |
| `post-update` | Warning is logged, but the container remains running |
| `post-check` | Warning is logged, no further action |

!!! danger
    A failing `pre-update` hook will prevent the container from being updated. Ensure your scripts are reliable and handle errors gracefully.

## Example: Database Backup Before Update

```yaml
services:
  postgres:
    image: postgres:16
    labels:
      - "com.updock.lifecycle.pre-update=/usr/local/bin/pg_dumpall -U postgres > /backups/pre-update.sql"
      - "com.updock.lifecycle.pre-update.timeout=300"
      - "com.updock.lifecycle.post-update=/usr/local/bin/pg_isready -U postgres"
      - "com.updock.lifecycle.post-update.timeout=30"
    volumes:
      - db-backups:/backups
```

## Example: Notify External Service

```yaml
services:
  api:
    image: myapi:latest
    labels:
      - "com.updock.lifecycle.pre-update=curl -X POST http://status.internal/api/maintenance/start"
      - "com.updock.lifecycle.post-update=curl -X POST http://status.internal/api/maintenance/end"
```

## Dry Run

When `--dry-run` is active, hooks are **not executed**. Updock logs which hooks *would* run instead.
