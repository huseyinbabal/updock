# Private Registry (ECR / GCR)

Authenticate with private registries using a Docker config.json file.

## AWS ECR

```bash
# Generate config.json with ECR credentials
aws ecr get-login-password --region us-east-1 | \
  docker login --username AWS --password-stdin \
  123456789.dkr.ecr.us-east-1.amazonaws.com
```

## Google GCR

```bash
# Use a service account key
cat key.json | docker login -u _json_key --password-stdin \
  https://gcr.io
```

## docker-compose.yml

```yaml
services:
  app:
    image: 123456789.dkr.ecr.us-east-1.amazonaws.com/myapp:latest

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - $HOME/.docker/config.json:/config.json:ro
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
```

Mount the host's `~/.docker/config.json` into the Updock container
at `/config.json`. Updock reads credentials from this file automatically.

!!! tip "ECR token expiry"
    ECR tokens expire after 12 hours. Use a cron job on the host to
    refresh the token and restart Updock, or use
    [amazon-ecr-credential-helper](https://github.com/awslabs/amazon-ecr-credential-helper)
    which refreshes automatically.
