# Arguments Reference

Updock supports configuration via CLI flags, environment variables, or a combination of both. Environment variables use the `UPDOCK_` prefix.

!!! tip "Secrets Support"
    Any environment variable can be suffixed with `_FILE` to read its value from a file. This is useful for Docker secrets:
    ```
    UPDOCK_HTTP_API_TOKEN_FILE=/run/secrets/api_token
    ```

## Docker Connection

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--docker-host` | `UPDOCK_DOCKER_HOST` | `string` | `unix:///var/run/docker.sock` |
| `--tls-verify` | `UPDOCK_TLS_VERIFY` | `bool` | `false` |
| `--docker-config` | `UPDOCK_DOCKER_CONFIG` | `string` | `~/.docker/config.json` |

## Scheduling

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--interval` | `UPDOCK_INTERVAL` | `int` (seconds) | `86400` |
| `--schedule` | `UPDOCK_SCHEDULE` | `string` (cron) | *none* |
| `--run-once` | `UPDOCK_RUN_ONCE` | `bool` | `false` |

!!! note
    `--schedule` takes precedence over `--interval` when both are set. The cron format supports 5 or 6 fields (with optional seconds).

## Container Selection

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--monitor-all` | `UPDOCK_MONITOR_ALL` | `bool` | `true` |
| `--label-enable` | `UPDOCK_LABEL_ENABLE` | `bool` | `false` |
| `--disable-containers` | `UPDOCK_DISABLE_CONTAINERS` | `string` (csv) | *none* |
| `--include-stopped` | `UPDOCK_INCLUDE_STOPPED` | `bool` | `false` |
| `--include-restarting` | `UPDOCK_INCLUDE_RESTARTING` | `bool` | `false` |
| `--revive-stopped` | `UPDOCK_REVIVE_STOPPED` | `bool` | `false` |
| `--scope` | `UPDOCK_SCOPE` | `string` | *none* |

## Update Behavior

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--cleanup` | `UPDOCK_CLEANUP` | `bool` | `false` |
| `--remove-volumes` | `UPDOCK_REMOVE_VOLUMES` | `bool` | `false` |
| `--stop-timeout` | `UPDOCK_STOP_TIMEOUT` | `int` (seconds) | `10` |
| `--dry-run` | `UPDOCK_DRY_RUN` | `bool` | `false` |
| `--no-pull` | `UPDOCK_NO_PULL` | `bool` | `false` |
| `--no-restart` | `UPDOCK_NO_RESTART` | `bool` | `false` |
| `--rolling-restart` | `UPDOCK_ROLLING_RESTART` | `bool` | `false` |
| `--label-precedence` | `UPDOCK_LABEL_PRECEDENCE` | `bool` | `false` |
| `--lifecycle-hooks` | `UPDOCK_LIFECYCLE_HOOKS` | `bool` | `false` |

## HTTP / Web UI

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--http-addr` | `UPDOCK_HTTP_ADDR` | `string` | `:8080` |
| `--http-enabled` | `UPDOCK_HTTP_ENABLED` | `bool` | `true` |
| `--http-api-token` | `UPDOCK_HTTP_API_TOKEN` | `string` | *none* |
| `--metrics` | `UPDOCK_METRICS` | `bool` | `false` |

## Notifications

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--webhook-url` | `UPDOCK_WEBHOOK_URL` | `string` (csv) | *none* |
| `--notification-template` | `UPDOCK_NOTIFICATION_TEMPLATE` | `string` | *built-in* |
| `--no-startup-message` | `UPDOCK_NO_STARTUP_MESSAGE` | `bool` | `false` |

## Policy & Audit

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--policy-file` | `UPDOCK_POLICY_FILE` | `string` | `updock.yml` |
| `--audit-log` | `UPDOCK_AUDIT_LOG` | `string` | `/var/lib/updock/audit.json` |

!!! tip "Policy file"
    The policy file (`updock.yml`) is Updock's core differentiator. It defines
    update strategies, maintenance windows, approval modes, and container groups.
    See [Policies](policies.md) for the full reference.

## Logging

| Argument | Environment Variable | Type | Default |
|---|---|---|---|
| `--log-level` | `UPDOCK_LOG_LEVEL` | `string` | `info` |
| `--warn-on-head-failure` | `UPDOCK_WARN_ON_HEAD_FAILURE` | `string` | `auto` |

!!! info "`--warn-on-head-failure`"
    Controls behavior when a `HEAD` request to a registry fails. Accepted values: `always`, `never`, `auto`. In `auto` mode, Updock falls back to a full `GET` request silently.

## Example

```bash
docker run -d \
  --name updock \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e UPDOCK_INTERVAL=3600 \
  -e UPDOCK_CLEANUP=true \
  -e UPDOCK_HTTP_API_TOKEN_FILE=/run/secrets/token \
  -e UPDOCK_LOG_LEVEL=debug \
  -p 8080:8080 \
  updock/updock
```
