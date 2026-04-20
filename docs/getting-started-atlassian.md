# Getting Started: Atlassian (Jira + Confluence)

Set up an Atlassian webhook pipeline: run relaymesh, create an Atlassian provider instance, complete OAuth, configure Jira/Confluence webhooks, and route matched events to your broker.

## Prerequisites

- Go 1.24+
- Docker + Docker Compose
- ngrok (for local dev): https://ngrok.com/download
- Atlassian Cloud site (Jira and/or Confluence)

## 1) Start dependencies

```bash
docker compose up -d
```

## 2) Expose relaymesh with ngrok

```bash
ngrok http 8080
```

## 3) Create Atlassian OAuth app

1. Create OAuth 2.0 (3LO) app in Atlassian developer console.
2. Add callback URL: `https://<your-ngrok-url>/auth/atlassian/callback`
3. Copy client ID and secret.
4. Add scopes (for example `read:jira-user`, `read:jira-work`, `read:confluence-content.summary`).

## 4) Configure relaymesh and start server

```yaml
server:
  port: 8080
endpoint: https://<your-ngrok-url>

storage:
  driver: postgres
  dsn: postgres://relaymesh:relaymesh@localhost:5432/relaymesh?sslmode=disable
  dialect: postgres
  auto_migrate: true
```

```bash
go run ./main.go serve --config config.yaml
```

## 5) Create Atlassian provider instance

`atlassian.yaml`:

```yaml
webhook:
  secret: your-atlassian-webhook-secret
oauth:
  client_id: your-atlassian-client-id
  client_secret: your-atlassian-client-secret
  scopes:
    - read:jira-user
    - read:jira-work
    - read:confluence-content.summary
api:
  base_url: https://auth.atlassian.com
  web_base_url: https://auth.atlassian.com
```

```bash
relaymesh --endpoint http://localhost:8080 providers create \
  --provider atlassian \
  --config-file atlassian.yaml
```

## 6) Complete OAuth installation

```bash
relaymesh --endpoint http://localhost:8080 providers list --provider atlassian
```

Open install URL:

```text
http://localhost:8080/?provider=atlassian&instance=<instance-hash>
```

## 7) Create driver + rules

```bash
relaymesh --endpoint http://localhost:8080 drivers create --name amqp --config-file amqp.yaml
```

Jira issue example:

```bash
relaymesh --endpoint http://localhost:8080 rules create \
  --when 'provider == "atlassian" && webhookEvent == "jira:issue_created"' \
  --emit atlassian.jira.issue.created \
  --driver-id <driver-id>
```

Confluence page example:

```bash
relaymesh --endpoint http://localhost:8080 rules create \
  --when 'provider == "atlassian" && contains(webhookEvent, "page")' \
  --emit atlassian.confluence.page \
  --driver-id <driver-id>
```

## 8) Configure webhook target

- Webhook URL: `https://<your-ngrok-url>/webhooks/atlassian`

## Notes

- Provider name is `atlassian` for both Jira and Confluence events.
- Namespace mapping:
  - Jira: project id/key
  - Confluence: space id/key
- Legacy `jira` alias is still accepted for backward compatibility.
