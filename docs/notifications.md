# Notifications

Updock can send webhook notifications when containers are updated, allowing integration with Slack, Discord, Microsoft Teams, and custom services.

## Webhook Configuration

Set one or more webhook URLs using the `--webhook-url` flag or environment variable:

```bash
docker run -d \
  -e UPDOCK_WEBHOOK_URL="https://hooks.slack.com/services/T00/B00/xxx" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  updock/updock
```

## Multiple Webhook URLs

Provide multiple URLs as a comma-separated list to notify several services simultaneously:

```bash
UPDOCK_WEBHOOK_URL="https://hooks.slack.com/services/xxx,https://discord.com/api/webhooks/yyy"
```

!!! info
    Each URL receives the same payload. Updock sends notifications to all configured endpoints in parallel.

## Notification Payload

Updock sends a JSON `POST` request with the following structure:

```json
{
  "text": "Updated container 'nginx' (nginx:1.24 → nginx:1.25)",
  "containers": [
    {
      "name": "nginx",
      "old_image": "nginx:1.24",
      "new_image": "nginx:1.25",
      "updated_at": "2025-01-15T10:30:00Z"
    }
  ]
}
```

The `text` field is compatible with Slack and Discord incoming webhooks out of the box.

## Custom Templates

Override the default message format with `--notification-template`:

```bash
UPDOCK_NOTIFICATION_TEMPLATE="Container {{.Name}} updated from {{.OldImage}} to {{.NewImage}}"
```

Available template variables:

| Variable | Description |
|---|---|
| `{{.Name}}` | Container name |
| `{{.OldImage}}` | Previous image reference |
| `{{.NewImage}}` | New image reference |
| `{{.UpdatedAt}}` | Timestamp of the update |
| `{{.Hostname}}` | Host where Updock is running |

!!! tip
    Templates use Go's `text/template` syntax. You can use conditionals, loops, and formatting functions.

### Multi-Container Template

When multiple containers are updated in a single cycle, the template receives a list:

```
{{range .Entries}}• {{.Name}}: {{.OldImage}} → {{.NewImage}}
{{end}}
```

## Startup Message

By default, Updock sends a notification when it starts, confirming the webhook is working:

```json
{
  "text": "Updock started monitoring 5 containers on host 'server-01'"
}
```

### Disabling Startup Messages

Suppress the startup notification with `--no-startup-message`:

```bash
docker run -d \
  -e UPDOCK_WEBHOOK_URL="https://hooks.slack.com/services/xxx" \
  -e UPDOCK_NO_STARTUP_MESSAGE=true \
  -v /var/run/docker.sock:/var/run/docker.sock \
  updock/updock
```

## Platform Examples

=== "Slack"

    ```bash
    UPDOCK_WEBHOOK_URL="https://hooks.slack.com/services/T00/B00/xxxxx"
    ```

=== "Discord"

    Append `/slack` to use Discord's Slack-compatible endpoint:

    ```bash
    UPDOCK_WEBHOOK_URL="https://discord.com/api/webhooks/123456/abcdef/slack"
    ```

=== "Microsoft Teams"

    ```bash
    UPDOCK_WEBHOOK_URL="https://outlook.office.com/webhook/xxxxx"
    ```

=== "Custom HTTP"

    Any endpoint that accepts a JSON `POST`:

    ```bash
    UPDOCK_WEBHOOK_URL="https://api.internal.example.com/updock-events"
    ```

## Dry Run

When `--dry-run` is active, notifications include a `[DRY RUN]` prefix to indicate no actual updates were performed.
