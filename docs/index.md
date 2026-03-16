# Updock

![GitHub Release](https://img.shields.io/github/v/release/huseyinbabal/updock?style=flat-square)
![GitHub Stars](https://img.shields.io/github/stars/huseyinbabal/updock?style=flat-square)
![GitHub Forks](https://img.shields.io/github/forks/huseyinbabal/updock?style=flat-square)
![GitHub License](https://img.shields.io/github/license/huseyinbabal/updock?style=flat-square)

**Declarative container update platform with policy engine and Web UI.**

Updock is not just another container auto-updater. It gives you full control
over **what** gets updated, **when**, and **how** — through a declarative
`updock.yml` policy file.

## Why Updock?

Traditional container updaters blindly pull and restart. Updock takes a
different approach:

- **Declarative policies** — Define update rules in `updock.yml`: patch-only,
  manual approval, maintenance windows, container groups.
- **Audit trail** — Every update, rollback, and approval is recorded in a
  queryable audit log.
- **Health-aware rollback** — If the new container fails its health check,
  Updock automatically rolls back.
- **Web UI dashboard** — Real-time status, update history, audit log, and
  manual controls at `http://localhost:8080`.

## Key Features

- **Policy engine** — Semver-aware strategies (`patch`, `minor`, `major`, `pin`),
  approval modes (`auto`, `manual`), and maintenance windows.
- **Audit log** — Persistent, append-only log of every action with timestamps,
  actors, and outcomes.
- **Container groups** — Coordinate updates across related containers with
  ordered rolling restarts.
- **Lifecycle hooks** — Run commands inside containers before and after updates.
- **REST API** — Full API for CI/CD integration, manual triggers, and audit queries.
- **Prometheus metrics** — Built-in `/metrics` endpoint.
- **Private registries** — Docker Hub, ECR, GCR, and any `config.json`-compatible registry.
- **Safe recreation** — Automatic rollback if the new container fails to start.

## Quick Start

=== "docker-compose (recommended)"

    ```yaml
    services:
      updock:
        image: ghcr.io/huseyinbabal/updock:latest
        volumes:
          - /var/run/docker.sock:/var/run/docker.sock
          - ./updock.yml:/etc/updock/updock.yml
        ports:
          - "8080:8080"
    ```

=== "docker run"

    ```bash
    docker run -d \
      --name updock \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v $(pwd)/updock.yml:/etc/updock/updock.yml \
      -p 8080:8080 \
      updock
    ```

Create an `updock.yml` alongside your `docker-compose.yml`:

```yaml
policies:
  default:
    strategy: patch        # only auto-apply patch updates
    approve: auto
    rollback: on-failure

containers:
  postgres:
    policy: locked         # never auto-update the database
    schedule: "02:00-04:00"

groups:
  web-stack:
    members: [redis, app, nginx]
    strategy: rolling
    order: [redis, app, nginx]
```

Then open [http://localhost:8080](http://localhost:8080) for the dashboard.
