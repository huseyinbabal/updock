# Zero-Downtime Rolling Update

Use lifecycle hooks and rolling restart to update a load-balanced web
application without dropping connections.

## docker-compose.yml

```yaml
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    labels:
      com.updock.depends-on: "app"

  app:
    image: myorg/api:latest
    labels:
      com.updock.lifecycle.pre-update: "/app/drain.sh"
      com.updock.lifecycle.post-update: "/app/healthcheck.sh"
      com.updock.lifecycle.pre-update-timeout: "2"
      com.updock.stop-signal: "SIGQUIT"

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    command: ["--lifecycle-hooks", "--rolling-restart"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"
```

## updock.yml

```yaml
policies:
  default:
    strategy: all
    approve: auto
    rollback: on-failure
    health_timeout: 45s

groups:
  web:
    members: [app, nginx]
    strategy: rolling
    order: [app, nginx]
```

## How it works

1. Updock detects a new `myorg/api` image
2. **Pre-update hook** runs `drain.sh` inside the app container (stops
   accepting new connections, waits for in-flight requests)
3. Container receives `SIGQUIT` for graceful shutdown
4. New container starts with updated image
5. **Post-update hook** runs `healthcheck.sh` to verify the new version
6. If health check fails, Updock rolls back automatically
7. Nginx is updated last (depends-on ordering)
