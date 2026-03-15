# Metrics

Updock exposes Prometheus-compatible metrics when `--metrics` is enabled. Metrics are served at `/metrics` on the HTTP address.

## Enabling Metrics

```bash
docker run -d \
  -e UPDOCK_METRICS=true \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  updock/updock
```

## Available Metrics

### Update Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_updates_total` | Counter | Total number of container updates performed |
| `updock_updates_failed_total` | Counter | Total number of failed update attempts |
| `updock_updates_skipped_total` | Counter | Total number of updates skipped (dry-run, hooks, etc.) |

### Scan Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_scans_total` | Counter | Total number of update check cycles completed |
| `updock_scan_duration_seconds` | Histogram | Duration of each update check cycle |
| `updock_last_scan_timestamp` | Gauge | Unix timestamp of the last completed scan |

### Container Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_containers_monitored` | Gauge | Number of containers currently being monitored |
| `updock_containers_outdated` | Gauge | Number of containers with available updates |
| `updock_container_update_available` | Gauge | Whether an update is available (per container label) |

### Image Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_image_pulls_total` | Counter | Total number of image pulls performed |
| `updock_image_pull_duration_seconds` | Histogram | Duration of image pull operations |
| `updock_old_images_cleaned_total` | Counter | Total number of old images removed (with `--cleanup`) |

### Hook Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_hook_executions_total` | Counter | Total lifecycle hook executions (labeled by stage) |
| `updock_hook_failures_total` | Counter | Total lifecycle hook failures (labeled by stage) |

### System Metrics

| Metric Name | Type | Description |
|---|---|---|
| `updock_info` | Gauge | Build info (version, commit labels) |
| `updock_uptime_seconds` | Gauge | Seconds since Updock started |

## Labels

Most metrics include the following Prometheus labels where applicable:

| Label | Description |
|---|---|
| `container` | Container name |
| `image` | Full image reference |
| `scope` | Updock scope (if configured) |
| `stage` | Hook stage (`pre-check`, `pre-update`, `post-update`, `post-check`) |

## Prometheus Scrape Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: "updock"
    scrape_interval: 30s
    static_configs:
      - targets: ["updock:8080"]
```

!!! tip
    The `/metrics` endpoint does not require Bearer token authentication, so no extra Prometheus configuration is needed for auth.

## Grafana Dashboard

You can visualize Updock metrics with queries like:

```promql
# Update rate over time
rate(updock_updates_total[1h])

# Containers currently outdated
updock_containers_outdated

# Average scan duration
histogram_quantile(0.95, rate(updock_scan_duration_seconds_bucket[5m]))
```
