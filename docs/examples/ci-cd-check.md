# CI/CD One-Shot Check

Use Updock in `--run-once` mode inside a CI pipeline to check if any
containers are running outdated images, without applying updates.

## GitHub Actions

```yaml
name: Image Freshness Check

on:
  schedule:
    - cron: '0 8 * * 1'  # every Monday at 8am

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - name: Check for outdated images
        run: |
          docker run --rm \
            -v /var/run/docker.sock:/var/run/docker.sock \
            ghcr.io/huseyinbabal/updock:latest \
            --run-once \
            --dry-run \
            --log-level info
```

## Shell Script

```bash
#!/bin/bash
# check-updates.sh - Run as a cron job on the host

docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/huseyinbabal/updock:latest \
  --run-once \
  --dry-run \
  --webhook-url "https://hooks.slack.com/services/XXX/YYY/ZZZ"
```

The `--run-once` flag performs a single check and exits.
Combined with `--dry-run`, it reports outdated images without
modifying anything. Add `--webhook-url` to get notified.
