# Container Selection

Updock provides several mechanisms to control which containers are monitored and updated.

## Default Behavior

By default, Updock monitors **all running containers** on the Docker host. This can be changed using labels, flags, or container name arguments.

## Enable / Disable Labels

### Opt-in Mode (`--label-enable`)

When `--label-enable` is set, Updock ignores all containers **unless** they carry the enable label:

```yaml
services:
  myapp:
    image: myapp:latest
    labels:
      - "com.updock.enable=true"
```

!!! warning
    With `--label-enable`, containers without the label are completely invisible to Updock — they will not be monitored or updated.

### Opt-out Mode (default)

In the default mode (`--monitor-all=true`), all containers are monitored. You can exclude specific containers with:

```yaml
services:
  database:
    image: postgres:16
    labels:
      - "com.updock.enable=false"
```

## Monitor-Only Label

To track image updates without applying them, use the monitor-only label:

```yaml
services:
  production-db:
    image: postgres:16
    labels:
      - "com.updock.monitor-only=true"
```

!!! info
    Monitor-only containers appear in the Web UI and API with available updates listed, but Updock will never restart or recreate them.

## Container Name Arguments

You can pass container names directly as positional arguments to limit a run to specific containers:

```bash
docker exec updock updock myapp nginx redis
```

This is particularly useful with `--run-once` for targeted, on-demand updates.

## Disable Containers Flag

The `--disable-containers` flag accepts a comma-separated list of container names to exclude:

```bash
docker run -d \
  -e UPDOCK_DISABLE_CONTAINERS="postgres,redis,mongo" \
  updock/updock
```

These containers will be skipped even if they have `com.updock.enable=true`.

## Scope Filtering

The `--scope` flag limits Updock to containers matching a specific scope label. See the [Multiple Instances](multiple-instances.md) guide for details.

## Stopped & Restarting Containers

By default, only **running** containers are considered.

| Flag | Effect |
|---|---|
| `--include-stopped` | Also monitor containers in `exited` state |
| `--include-restarting` | Also monitor containers in `restarting` state |
| `--revive-stopped` | Restart stopped containers after updating their image |

!!! tip
    `--revive-stopped` implies `--include-stopped`. Use it when you want Updock to bring back containers that were stopped but have a newer image available.

## Policy-Based Selection (updock.yml)

In addition to labels and flags, containers can be configured in the
`updock.yml` policy file. This is the recommended approach for complex setups:

```yaml
containers:
  nginx:
    policy: conservative    # assign a named policy
    schedule: "02:00-05:00" # maintenance window

  postgres:
    policy: locked          # never auto-update

  legacy-app:
    ignore: true            # completely invisible to Updock
```

Containers marked with `ignore: true` in the policy file are skipped entirely,
regardless of labels or flags. See [Policies](policies.md) for full details.

## Precedence Summary

The selection logic is evaluated in order:

1. **Policy file ignore** — `ignore: true` in `updock.yml` always excludes
2. **Name arguments** — if names are provided, only those containers are included
3. **Disable list** — container must not be in `--disable-containers`
4. **Opt-out label** — `com.updock.disable=true` always excludes
5. **Scope filter** — container must match `--scope` (if set)
6. **Label mode** — if `--label-enable`, container must have `com.updock.enable=true`
7. **State filter** — container state must be eligible (running, stopped, restarting)
8. **Policy strategy** — `strategy: pin` prevents auto-update (requires approval)
9. **Maintenance window** — `schedule` in policy file restricts update times
