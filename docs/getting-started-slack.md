# Getting Started: Slack

Set up a Slack webhook pipeline: run relaymesh, register a Slack provider instance, configure Slack Event Subscriptions, and publish Slack events to your broker.

## Prerequisites

- Go 1.24+
- Docker + Docker Compose
- ngrok (for local dev): https://ngrok.com/download
- A Slack workspace where you can create apps

## 1) Start dependencies

```bash
docker compose up -d
```

## 2) Expose relaymesh with ngrok (local only)

```bash
ngrok http 8080
```

Copy the HTTPS forwarding URL (example: `https://abc123.ngrok-free.app`). Keep ngrok running.

## 3) Create a Slack app

1. Go to https://api.slack.com/apps
2. Click **Create New App** (from scratch)
3. Pick your workspace
4. Open **Basic Information** and copy:
   - **Signing Secret**
5. Open **OAuth & Permissions**:
   - Add **Bot Token Scopes** you need (example: `app_mentions:read`, `channels:history`, `chat:write`)
   - Copy **Client ID** and **Client Secret**
6. Open **Event Subscriptions**:
   - Enable events
   - Set **Request URL** to: `https://<your-ngrok-url>/webhooks/slack`
   - Subscribe to bot events you want (for example `message.channels`, `app_mention`)
7. Install/reinstall app to workspace if Slack prompts for it

## 4) Configure relaymesh

`config.yaml`:

```yaml
server:
  port: 8080
endpoint: https://<your-ngrok-url>

storage:
  driver: postgres
  dsn: postgres://relaymesh:relaymesh@localhost:5432/relaymesh?sslmode=disable
  dialect: postgres
  auto_migrate: true

auth:
  oauth2:
    enabled: false

redirect_base_url: https://app.example.com/success
```

Start the server:

```bash
go run ./main.go serve --config config.yaml
```

## 5) Create Slack provider instance

Create `slack.yaml`:

```yaml
webhook:
  secret: your-slack-signing-secret
oauth:
  client_id: your-slack-client-id
  client_secret: your-slack-client-secret
  scopes:
    - app_mentions:read
    - channels:history
    - chat:write
```

Create the instance:

```bash
relaymesh --endpoint http://localhost:8080 providers create \
  --provider slack \
  --config-file slack.yaml
```

## 6) Complete OAuth installation

Get the provider instance hash:

```bash
relaymesh --endpoint http://localhost:8080 providers list --provider slack
```

Open the install URL:

```text
http://localhost:8080/?provider=slack&instance=<instance-hash>
```

Authorize the Slack app. relaymesh stores the workspace installation/token after callback.

## 7) Create a driver + rule

```bash
relaymesh --endpoint http://localhost:8080 drivers create --name amqp --config-file amqp.yaml
```

Create a rule for `app_mention` events:

```bash
relaymesh --endpoint http://localhost:8080 rules create \
  --when 'provider == "slack" && event == "event_callback.app_mention"' \
  --emit slack.app_mention \
  --driver-id <driver-id>
```

`--driver-id` is the driver record ID (see `relaymesh drivers list`).

## 8) Verify with Slack

Post in a channel where the app is present (for example mention the bot). Slack should send the event to:

```text
https://<your-ngrok-url>/webhooks/slack
```

If the event matches your rule, relaymesh publishes it to your configured broker topic.

## 9) Local manual test (without Slack)

You can send a signed test payload manually.

Create `slack-event.json`:

```json
{
  "type": "event_callback",
  "team_id": "T123",
  "event": {
    "type": "app_mention",
    "channel": "C123",
    "user": "U123",
    "text": "hello relaymesh"
  }
}
```

Send it with valid Slack signature headers:

```bash
ts=$(date +%s)
body=$(cat slack-event.json)
sig=$(printf 'v0:%s:%s' "$ts" "$body" | openssl dgst -sha256 -hmac "$SLACK_SIGNING_SECRET" | sed 's/^.* //')

curl -X POST http://localhost:8080/webhooks/slack \
  -H "Content-Type: application/json" \
  -H "X-Slack-Request-Timestamp: $ts" \
  -H "X-Slack-Signature: v0=$sig" \
  -d "$body"
```

## Notes

- Slack request verification uses HMAC SHA256 and a 5-minute timestamp window.
- `url_verification` requests are handled automatically by the Slack webhook endpoint.
- OAuth callback path is `/auth/slack/callback`.
- Installation data is stored using the Slack workspace/team id as `account_id` and `installation_id`.
