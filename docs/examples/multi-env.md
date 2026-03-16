# Multi-Environment Fleet

Run separate Updock instances for staging and production on the same
Docker host using scope isolation.

## docker-compose.yml

```yaml
services:
  # --- Staging ---
  staging-app:
    image: myorg/app:latest
    labels:
      com.updock.scope: staging

  staging-db:
    image: postgres:16
    labels:
      com.updock.scope: staging

  updock-staging:
    image: ghcr.io/huseyinbabal/updock:latest
    command: ["--scope", "staging", "--interval", "10m"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock-staging.yml:/etc/updock/updock.yml
    ports:
      - "8081:8080"
    labels:
      com.updock.disable: "true"

  # --- Production ---
  prod-app:
    image: myorg/app:latest
    labels:
      com.updock.scope: production

  prod-db:
    image: postgres:16
    labels:
      com.updock.scope: production

  updock-production:
    image: ghcr.io/huseyinbabal/updock:latest
    command: ["--scope", "production", "--interval", "1h"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock-production.yml:/etc/updock/updock.yml
    ports:
      - "8082:8080"
    labels:
      com.updock.disable: "true"
```

## updock-staging.yml

```yaml
policies:
  default:
    strategy: all
    approve: auto
```

## updock-production.yml

```yaml
policies:
  default:
    strategy: patch
    approve: manual
    rollback: on-failure

containers:
  prod-db:
    policy: locked
    schedule: "02:00-04:00"
```

Each Updock instance only sees containers with its matching scope label.
Staging updates aggressively; production only auto-applies patch updates
and requires approval for the database.
