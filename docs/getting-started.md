# Getting Started

## Installation

### Docker (recommended)

```bash
docker run -d \
  --name updock \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd)/updock.yml:/etc/updock/updock.yml \
  -p 8080:8080 \
  updock
```

### Binary

Download the latest binary from the [releases page](https://github.com/huseyinbabal/updock/releases) and run:

```bash
updock --policy-file updock.yml
```

### Build from source

```bash
git clone https://github.com/huseyinbabal/updock.git
cd updock
make build
./bin/updock
```

## Create Your Policy File

Updock's core differentiator is the `updock.yml` policy file. Create one
in the same directory as your `docker-compose.yml`:

```yaml
# updock.yml
policies:
  default:
    strategy: all          # allow any update
    approve: auto          # apply immediately
    rollback: on-failure   # rollback if health check fails

  conservative:
    strategy: patch        # only patch versions (1.2.3 -> 1.2.4)
    approve: auto
    rollback: on-failure

  locked:
    strategy: pin          # never auto-update
    approve: manual        # require approval via Web UI

containers:
  nginx:
    policy: conservative
    schedule: "02:00-05:00"  # only update between 2am-5am

  postgres:
    policy: locked           # never auto-update the database

  legacy-app:
    ignore: true             # completely skip this container

groups:
  web-stack:
    members: [redis, app, nginx]
    strategy: rolling
    order: [redis, app, nginx]
```

## Configuration

Updock supports three configuration layers:

1. **`updock.yml`** — Declarative policies, container overrides, groups
2. **CLI flags** — Runtime behavior (`--interval`, `--http-addr`, etc.)
3. **Environment variables** — Same as flags with `UPDOCK_` prefix

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--policy-file` | `UPDOCK_POLICY_FILE` | Policy file path | `updock.yml` |
| `--interval` | `UPDOCK_INTERVAL` | Polling interval | `5m` |
| `--schedule` | `UPDOCK_SCHEDULE` | Cron expression (6-field) | — |
| `--http-addr` | `UPDOCK_HTTP_ADDR` | Web UI listen address | `:8080` |
| `--audit-log` | `UPDOCK_AUDIT_LOG` | Audit log file path | `/var/lib/updock/audit.json` |
| `--log-level` | `UPDOCK_LOG_LEVEL` | Log verbosity | `info` |

See [Arguments](arguments.md) for the full list.

## How It Works

1. **Discover** — Updock lists containers, filtered by policy file and labels.
2. **Evaluate** — The policy engine checks strategy, maintenance window, and approval mode.
3. **Check** — Compares local image digest with the remote registry.
4. **Approve** — Auto-approved updates proceed; manual-approval updates are queued.
5. **Update** — Pulls the new image, runs lifecycle hooks, recreates the container.
6. **Verify** — Waits for health check. If it fails, rolls back automatically.
7. **Record** — Every action is written to the audit log.
8. **Notify** — Webhook notifications are sent about the result.
