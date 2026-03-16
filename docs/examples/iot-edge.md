# IoT Edge Devices

Run Updock on resource-constrained edge devices with minimal bandwidth
usage and strict update control.

## docker-compose.yml

```yaml
services:
  sensor-collector:
    image: myorg/sensor-collector:latest

  mqtt-broker:
    image: eclipse-mosquitto:2

  edge-gateway:
    image: myorg/edge-gateway:latest

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    command: [
      "--interval", "6h",
      "--http-addr", ":8080",
      "--log-level", "warn"
    ]
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
    strategy: digest
    approve: auto
    rollback: on-failure
    health_timeout: 120s

  critical:
    strategy: pin
    approve: manual

containers:
  mqtt-broker:
    policy: critical
  edge-gateway:
    policy: critical
  sensor-collector:
    schedule: "02:00-04:00"
```

Key considerations for edge:

- **Long interval** (`6h`) to minimize bandwidth and registry API calls
- **Digest strategy** for custom images (no semver tags)
- **Pin critical services** (MQTT broker, gateway) to prevent unexpected restarts
- **Longer health timeout** (`120s`) for slow-starting containers on limited hardware
- **Maintenance window** on sensor collector to avoid data gaps during collection cycles
