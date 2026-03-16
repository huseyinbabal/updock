# Linked Containers

Updock can manage update ordering between dependent containers using the `com.updock.depends-on` label and implicit network dependencies.

## Dependency Label

Use `com.updock.depends-on` to declare that a container depends on one or more other containers:

```yaml
services:
  api:
    image: myapi:latest
    labels:
      - "com.updock.depends-on=postgres,redis"

  postgres:
    image: postgres:16

  redis:
    image: redis:7
```

When Updock updates containers, it respects the declared dependency order:

1. Dependencies are updated **first** (e.g., `postgres`, `redis`)
2. The dependent container is updated **after** its dependencies are healthy

!!! info
    Multiple dependencies are specified as a comma-separated list.

## Dependency Ordering

Updock builds a directed acyclic graph (DAG) from the dependency labels and processes updates in topological order:

```
postgres → api → nginx
redis   ↗
```

In this example, `postgres` and `redis` are updated first (in parallel if possible), then `api`, then `nginx`.

!!! danger "Circular Dependencies"
    Circular dependencies (e.g., A depends on B, B depends on A) are detected at startup and cause Updock to log an error and skip those containers.

## Implicit Dependencies via `network_mode`

Containers using `network_mode: "container:<name>"` have an implicit dependency on the referenced container. Updock detects this automatically:

```yaml
services:
  vpn:
    image: wireguard:latest

  app:
    image: myapp:latest
    network_mode: "container:vpn"
```

Here, `app` is implicitly dependent on `vpn`. If `vpn` is updated and restarted, `app` is restarted afterward to restore network connectivity.

## Rolling Restart

When `--rolling-restart` is enabled, Updock restarts containers one at a time rather than stopping all and starting all. This is particularly useful with linked containers:

```bash
docker run -d \
  -e UPDOCK_ROLLING_RESTART=true \
  -v /var/run/docker.sock:/var/run/docker.sock \
  updock/updock
```

With rolling restart and dependencies:

1. Each container is fully stopped, updated, and restarted before moving to the next
2. Dependency order is still respected
3. Dependent containers wait for their dependencies to be healthy before proceeding

!!! tip
    Combine `--rolling-restart` with health checks on your containers for zero-downtime updates in dependency chains.

## Stop Timeout

When stopping a container for update, Updock waits for the configured stop timeout before force-killing:

```yaml
services:
  api:
    image: myapi:latest
    labels:
      - "com.updock.depends-on=postgres"
      - "com.updock.stop-timeout=30"
```

The per-container label overrides the global `--stop-timeout` flag.

## Example: Full Stack

```yaml
services:
  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - UPDOCK_ROLLING_RESTART=true
      - UPDOCK_CLEANUP=true

  db:
    image: postgres:16
    labels:
      - "com.updock.lifecycle.pre-update=pg_dumpall -U postgres > /backup/dump.sql"

  cache:
    image: redis:7

  api:
    image: myapi:latest
    labels:
      - "com.updock.depends-on=db,cache"

  web:
    image: myweb:latest
    labels:
      - "com.updock.depends-on=api"
```

Update order: `db` + `cache` → `api` → `web`.
