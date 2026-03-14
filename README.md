# relaymesh ⚡

> **⚠️ Warning:** Research and development only. Not production-ready.

relaymesh is a multi-tenant webhook router for GitHub, GitLab, and Bitbucket. It receives webhook events, evaluates rules, and publishes matching events to AMQP, NATS, or Kafka. Workers subscribe to those topics and can request SCM clients from the server.

## Core concepts

- Provider instance: a per-tenant provider configuration (OAuth + webhook secret + optional enterprise URLs).
- Driver: a broker configuration (AMQP/NATS/Kafka) stored per tenant.
- Rule: a `when` expression plus `emit` topic(s) targeting a driver.
- Worker: consumes published events and can request SCM clients from the server.
- Tenant: logical workspace selected by `X-Tenant-ID` or `--tenant-id`.
- Event log: stored webhook headers/body plus a body hash for auditing.

## Install 🧰

```bash
brew install relaymesh/homebrew-formula/githook
```

Or from source:

```bash
go build -o githook ./main.go
```

## Quick start (local) 🚀

1) Start the server:

```bash
githook serve --config config.yaml
```

Minimal `config.yaml`:

```yaml
server:
  port: 8080

endpoint: http://localhost:8080

storage:
  driver: postgres
  dsn: postgres://githook:githook@localhost:5432/githook?sslmode=disable
  dialect: postgres
  auto_migrate: true
```

2) Register a provider instance (YAML):

```bash
githook --endpoint http://localhost:8080 providers create \
  --provider github \
  --config-file github.yaml
```

Example `github.yaml`:

```yaml
app:
  app_id: 12345
  private_key_path: ./github.pem
oauth:
  client_id: your-client-id
  client_secret: your-client-secret
webhook:
  secret: your-webhook-secret
```

3) Create a driver config (YAML):

```bash
githook --endpoint http://localhost:8080 drivers create \
  --name amqp \
  --config-file amqp.yaml
```

Example `amqp.yaml`:

```yaml
url: amqp://guest:guest@localhost:5672/
exchange: githook.events
routing_key_template: "{topic}"
```

4) Create a rule:

```bash
githook --endpoint http://localhost:8080 rules create \
  --when 'action == "opened"' \
  --emit pr.opened \
  --driver-id <driver-id>
```

`--driver-id` is the driver record ID (see `githook drivers list`).

5) Point your provider webhook to:

```
http://<server-host>/webhooks/github
http://<server-host>/webhooks/gitlab
http://<server-host>/webhooks/bitbucket
```

## CLI essentials 🧭

- Providers: `providers list|get|create|update|delete`
- Drivers: `drivers list|get|create|update|delete`
- Rules: `rules list|get|create|update|delete|match`
- Namespaces: `namespaces list|update` and `namespaces webhook get|update`
- Installations: `installations list|get`

## Worker SDKs (rule id) 🛠️

Go:

```go
wk := worker.New(
  worker.WithEndpoint("http://localhost:8080"),
  worker.WithClientProvider(worker.NewRemoteSCMClientProvider()),
  worker.WithConcurrency(4),
  worker.WithRetryCount(1),
)

wk.HandleRule("<rule-id>", func(ctx context.Context, evt *worker.Event) error {
  switch strings.ToLower(evt.Provider) {
  case "github":
    _, _ = worker.GitHubClient(evt)
  case "gitlab":
    _, _ = worker.GitLabClient(evt)
  case "bitbucket":
    _, _ = worker.BitbucketClient(evt)
  }
	return nil
})

_ = wk.Run(ctx)
```

TypeScript:

```ts
import {
  New,
  WithEndpoint,
  WithClientProvider,
  WithConcurrency,
  WithRetryCount,
  NewRemoteSCMClientProvider,
  GitHubClient,
  GitLabClient,
  BitbucketClient,
} from "@relaymesh/sdk";

const worker = New(
  WithEndpoint("http://localhost:8080"),
  WithClientProvider(NewRemoteSCMClientProvider()),
  WithConcurrency(4),
  WithRetryCount(1),
);

worker.HandleRule("<rule-id>", async (_ctx, evt) => {
  switch ((evt.provider || "").toLowerCase()) {
    case "github":
      GitHubClient(evt);
      break;
    case "gitlab":
      GitLabClient(evt);
      break;
    case "bitbucket":
      BitbucketClient(evt);
      break;
  }
});

await worker.Run();
```

Python:

```python
from relaymesh import (
    New,
    WithEndpoint,
    WithClientProvider,
    WithConcurrency,
    NewRemoteSCMClientProvider,
    GitHubClient,
    GitLabClient,
    BitbucketClient,
)

wk = New(
    WithEndpoint("http://localhost:8080"),
    WithClientProvider(NewRemoteSCMClientProvider()),
    WithConcurrency(4),
)

def handler(ctx, evt):
    provider = (evt.provider or "").lower()
    if provider == "github":
        GitHubClient(evt)
    elif provider == "gitlab":
        GitLabClient(evt)
    elif provider == "bitbucket":
        BitbucketClient(evt)

wk.HandleRule("<rule-id>", handler)

wk.Run()
```

If a handler returns an error (Go) or throws/raises an exception (TypeScript/Python), the SDK marks the event log status as `failed`; otherwise it is marked `success`.
Use `WithRetryCount(n)` and `WithConcurrency(n)` in each SDK to control retry attempts and in-flight message processing.

## Docs index 📚

- Getting started: `docs/getting-started-github.md`, `docs/getting-started-gitlab.md`, `docs/getting-started-bitbucket.md`
- CLI: `docs/cli.md`
- Rules: `docs/rules.md`
- Drivers: `docs/drivers.md`
- Auth: `docs/auth.md`
- Events: `docs/events.md`
- Observability: `docs/observability.md`
- SDK clients: `docs/sdk_clients.md`
