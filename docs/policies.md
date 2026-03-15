# Policies

The policy engine is Updock's core feature. It replaces ad-hoc label-based
configuration with a declarative `updock.yml` file that defines **how** each
container should be updated.

## Policy File

Place `updock.yml` next to your `docker-compose.yml` or mount it at
`/etc/updock/updock.yml` in the Updock container:

```yaml
policies:
  default:
    strategy: all
    approve: auto
    rollback: on-failure
    health_timeout: 30s
```

## Strategies

The `strategy` field controls which image changes trigger an update:

| Strategy | Description |
|----------|-------------|
| `all` | Any image change (tag or digest). Most permissive. |
| `major` | Semver major, minor, and patch updates. |
| `minor` | Semver minor and patch updates only. |
| `patch` | Semver patch updates only. Safest for auto-updates. |
| `digest` | Same tag, different digest (e.g. rebuilt `latest`). |
| `pin` | Never auto-update. Requires manual approval. |

!!! example "Strategy examples"
    With `strategy: patch`, the image `nginx:1.25.3` would be updated to
    `nginx:1.25.4` but **not** to `nginx:1.26.0` or `nginx:2.0.0`.

## Approval Modes

The `approve` field controls whether updates are applied automatically:

| Mode | Description |
|------|-------------|
| `auto` | Apply updates immediately without human intervention. |
| `manual` | Queue updates for approval via the Web UI or API. |

When an update requires manual approval, it appears in the audit log with
type `approval.pending`. Use the Web UI or `POST /api/approve` to approve
or deny.

## Rollback Modes

The `rollback` field controls automatic rollback behavior:

| Mode | Description |
|------|-------------|
| `on-failure` | Rollback if the new container fails to start or health check. |
| `never` | Leave failed containers for manual intervention. |

The `health_timeout` field specifies how long to wait for the container's
health check to pass before triggering a rollback (default: 30s).

## Per-Container Overrides

Assign policies to specific containers and set maintenance windows:

```yaml
containers:
  nginx:
    policy: default
    schedule: "02:00-05:00"    # only update between 2am and 5am

  postgres:
    policy: locked             # use the "locked" policy

  legacy-app:
    ignore: true               # completely skip this container
```

## Maintenance Windows

The `schedule` field restricts updates to a time window in `HH:MM-HH:MM`
format (24-hour). Updates detected outside the window are deferred.

```yaml
containers:
  nginx:
    schedule: "02:00-05:00"    # 2am to 5am
  batch-jobs:
    schedule: "22:00-06:00"    # crosses midnight: 10pm to 6am
```

!!! tip
    Maintenance windows work together with strategies. A container with
    `strategy: patch` and `schedule: "02:00-05:00"` will only receive
    patch updates during the 2am-5am window.

## Container Groups

Groups coordinate updates across related containers:

```yaml
groups:
  web-stack:
    members: [redis, app, nginx]
    strategy: rolling
    order: [redis, app, nginx]
```

| Field | Description |
|-------|-------------|
| `members` | Container names in this group. |
| `strategy` | `parallel` (all at once) or `rolling` (one at a time). |
| `order` | Restart order for rolling updates. |

## Named Policies

Define reusable policies and reference them by name:

```yaml
policies:
  default:
    strategy: all
    approve: auto
    rollback: on-failure

  conservative:
    strategy: patch
    approve: auto
    rollback: on-failure

  locked:
    strategy: pin
    approve: manual

containers:
  nginx:
    policy: conservative
  postgres:
    policy: locked
  redis:
    policy: default
```

Containers without an explicit policy assignment use the `default` policy.
