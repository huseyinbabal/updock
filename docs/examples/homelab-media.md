# Homelab Media Server

Keep your Plex, Sonarr, Radarr, and other media containers up to date
automatically during off-peak hours.

## docker-compose.yml

```yaml
services:
  plex:
    image: linuxserver/plex:latest
    ports:
      - "32400:32400"
    volumes:
      - plex-config:/config
      - /mnt/media:/data

  sonarr:
    image: linuxserver/sonarr:latest
    ports:
      - "8989:8989"
    volumes:
      - sonarr-config:/config

  radarr:
    image: linuxserver/radarr:latest
    ports:
      - "7878:7878"
    volumes:
      - radarr-config:/config

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"

volumes:
  plex-config:
  sonarr-config:
  radarr-config:
```

## updock.yml

```yaml
policies:
  default:
    strategy: all
    approve: auto
    rollback: on-failure
    health_timeout: 60s

containers:
  plex:
    schedule: "03:00-06:00"
  sonarr:
    schedule: "03:00-06:00"
  radarr:
    schedule: "03:00-06:00"
```

Updates happen between 3am-6am so downloads and streams are not interrupted.
