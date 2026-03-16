# Web App with Database

Auto-update the app container but lock the database to manual-only
upgrades with maintenance windows.

## docker-compose.yml

```yaml
services:
  app:
    image: myorg/webapp:latest
    ports:
      - "3000:3000"
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
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

  database:
    strategy: patch
    approve: manual
    rollback: on-failure

containers:
  postgres:
    policy: database
    schedule: "02:00-04:00"
  redis:
    policy: default

groups:
  web-stack:
    members: [redis, app]
    strategy: rolling
    order: [redis, app]
```

The app and Redis update automatically. Postgres only receives patch updates
and requires manual approval via the Web UI.
