# Microservices with Dependencies

Manage a microservice architecture with ordered rolling updates
respecting service dependencies.

## docker-compose.yml

```yaml
services:
  postgres:
    image: postgres:16-alpine
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine

  user-service:
    image: myorg/user-service:latest
    labels:
      com.updock.depends-on: "postgres,redis"

  order-service:
    image: myorg/order-service:latest
    labels:
      com.updock.depends-on: "postgres,redis,user-service"

  notification-service:
    image: myorg/notification-service:latest
    labels:
      com.updock.depends-on: "redis"

  api-gateway:
    image: myorg/api-gateway:latest
    ports:
      - "8000:8000"
    labels:
      com.updock.depends-on: "user-service,order-service,notification-service"

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    command: ["--rolling-restart", "--lifecycle-hooks"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"

volumes:
  pgdata:
```

## updock.yml

```yaml
policies:
  default:
    strategy: all
    approve: auto
    rollback: on-failure

  infrastructure:
    strategy: patch
    approve: manual
    rollback: on-failure

containers:
  postgres:
    policy: infrastructure
    schedule: "02:00-04:00"
  redis:
    policy: infrastructure
    schedule: "02:00-04:00"

groups:
  backend:
    members: [user-service, order-service, notification-service, api-gateway]
    strategy: rolling
    order: [user-service, notification-service, order-service, api-gateway]
```

## Update order

When updates are available, Updock resolves the dependency graph
and restarts services in this order:

1. `user-service` (depends on postgres, redis)
2. `notification-service` (depends on redis)
3. `order-service` (depends on postgres, redis, user-service)
4. `api-gateway` (depends on all services)

Infrastructure (postgres, redis) requires manual approval and only
updates during the 2am-4am maintenance window.
