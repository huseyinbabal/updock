# Monitoring Stack

Keep Prometheus, Grafana, and Alertmanager updated with conservative
policies and Updock's own metrics scraped by Prometheus.

## docker-compose.yml

```yaml
services:
  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prom-data:/prometheus

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana

  alertmanager:
    image: prom/alertmanager:latest
    ports:
      - "9093:9093"

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"

volumes:
  prom-data:
  grafana-data:
```

## updock.yml

```yaml
policies:
  default:
    strategy: patch
    approve: auto
    rollback: on-failure

containers:
  prometheus:
    schedule: "04:00-05:00"
  grafana:
    schedule: "04:00-05:00"
  alertmanager:
    schedule: "04:00-05:00"
```

## prometheus.yml (scrape config)

```yaml
scrape_configs:
  - job_name: updock
    scrape_interval: 30s
    static_configs:
      - targets: ['updock:8080']
```

Prometheus scrapes Updock's `/metrics` endpoint. Create a Grafana
dashboard to visualize `updock_containers_updated_total`,
`updock_check_duration_seconds`, and `updock_monitored_containers`.
