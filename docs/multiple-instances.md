# Multiple Instances

When running multiple Updock instances on the same Docker host, the `--scope` flag prevents them from interfering with each other.

## The Problem

Without scoping, two Updock instances would both monitor and attempt to update the same containers, causing conflicts and duplicate operations.

## Setting a Scope

Assign a unique scope to each Updock instance:

```yaml
services:
  updock-production:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=production
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  updock-staging:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=staging
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

## Labeling Containers

Containers declare which scope they belong to using the `com.updock.scope` label:

```yaml
services:
  prod-api:
    image: myapi:latest
    labels:
      - "com.updock.scope=production"

  staging-api:
    image: myapi:staging
    labels:
      - "com.updock.scope=staging"
```

!!! important
    An Updock instance with `--scope=production` **only** monitors containers that have `com.updock.scope=production`. Containers without a scope label — or with a different scope — are ignored.

## Scope Matching Rules

| Updock `--scope` | Container Label | Monitored? |
|---|---|---|
| `production` | `com.updock.scope=production` | Yes |
| `production` | `com.updock.scope=staging` | No |
| `production` | *(no scope label)* | No |
| *(not set)* | `com.updock.scope=production` | Yes |
| *(not set)* | *(no scope label)* | Yes |

!!! note
    When `--scope` is **not set**, Updock monitors all containers regardless of their scope label. This is the default behavior.

## Opting Out: `scope=none`

A container can explicitly opt out of all scoped Updock instances by setting:

```yaml
labels:
  - "com.updock.scope=none"
```

A container with `scope=none` is **only** picked up by an Updock instance that has no `--scope` flag set. Any scoped instance will skip it.

## Self-Scoping

The Updock container itself should carry the scope label to ensure it is managed by the correct instance (or excluded entirely):

```yaml
services:
  updock-prod:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=production
    labels:
      - "com.updock.scope=production"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

!!! tip
    If you don't want Updock to update itself, use `com.updock.enable=false` on the Updock container instead.

## Example: Three Environments

```yaml
services:
  # Updock instances
  updock-prod:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=prod
      - UPDOCK_INTERVAL=86400
    labels:
      - "com.updock.scope=prod"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  updock-staging:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=staging
      - UPDOCK_INTERVAL=3600
    labels:
      - "com.updock.scope=staging"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  updock-dev:
    image: updock/updock
    environment:
      - UPDOCK_SCOPE=dev
      - UPDOCK_INTERVAL=300
    labels:
      - "com.updock.scope=dev"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  # Application containers
  api-prod:
    image: myapi:latest
    labels:
      - "com.updock.scope=prod"

  api-staging:
    image: myapi:staging
    labels:
      - "com.updock.scope=staging"

  api-dev:
    image: myapi:dev
    labels:
      - "com.updock.scope=dev"
```

Each Updock instance operates independently, with its own schedule and scope.
