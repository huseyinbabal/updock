# Private Registries

Updock can pull images from private registries by sharing Docker credentials from the host or using credential helpers.

## Sharing Docker Credentials

The simplest method is to mount the host's Docker config file into the Updock container:

```bash
docker run -d \
  --name updock \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v ~/.docker/config.json:/config.json:ro \
  -e UPDOCK_DOCKER_CONFIG=/config.json \
  updock/updock
```

!!! tip
    Mount the config as **read-only** (`:ro`) since Updock only needs to read credentials, never write them.

## Docker Login

If you run `docker login` on the host, credentials are stored in `~/.docker/config.json`. You can share this file directly:

```yaml
services:
  updock:
    image: updock/updock
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /home/deploy/.docker/config.json:/config.json:ro
    environment:
      - UPDOCK_DOCKER_CONFIG=/config.json
```

The config file typically looks like:

```json
{
  "auths": {
    "registry.example.com": {
      "auth": "dXNlcm5hbWU6cGFzc3dvcmQ="
    }
  }
}
```

## `--docker-config` Flag

Use this flag to specify a custom path to the Docker config file:

```bash
updock --docker-config /etc/updock/registry-auth.json
```

| Argument | Environment Variable | Default |
|---|---|---|
| `--docker-config` | `UPDOCK_DOCKER_CONFIG` | `~/.docker/config.json` |

## Credential Helpers

Updock supports Docker credential helpers for dynamic token retrieval. Configure the helper in your `config.json`:

=== "Amazon ECR"

    ```json
    {
      "credHelpers": {
        "123456789.dkr.ecr.us-east-1.amazonaws.com": "ecr-login"
      }
    }
    ```

    Make sure `docker-credential-ecr-login` is available inside the Updock container. Mount it as a volume or build a custom image:

    ```dockerfile
    FROM updock/updock
    COPY --from=amazon/amazon-ecr-credential-helper /usr/local/bin/docker-credential-ecr-login /usr/local/bin/
    ```

    You must also provide AWS credentials via environment variables or instance role.

=== "Google GCR / Artifact Registry"

    ```json
    {
      "credHelpers": {
        "gcr.io": "gcloud",
        "us-docker.pkg.dev": "gcloud"
      }
    }
    ```

    Alternatively, use a service account key:

    ```json
    {
      "auths": {
        "gcr.io": {
          "auth": "<base64 of _json_key:<service-account-json>>"
        }
      }
    }
    ```

=== "Azure ACR"

    ```json
    {
      "credHelpers": {
        "myregistry.azurecr.io": "acr-login"
      }
    }
    ```

!!! warning
    When using credential helpers, the helper binary must be accessible inside the Updock container at runtime. Mount it from the host or extend the Updock image.

## Multiple Registries

A single `config.json` can contain credentials for multiple registries:

```json
{
  "auths": {
    "registry.example.com": { "auth": "..." },
    "ghcr.io": { "auth": "..." }
  },
  "credHelpers": {
    "123456789.dkr.ecr.us-east-1.amazonaws.com": "ecr-login"
  }
}
```

Updock resolves credentials by matching the image's registry hostname against the entries in the config file.
