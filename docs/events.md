# Event Compatibility

relaymesh preserves provider event names in `Event.Name` and sets `Event.Provider` to the source system. Rules should target payload fields, not provider-specific envelope fields.

Rules evaluate a flattened view of the JSON payload, so fields like `action` and `pull_request.title` are accessible directly in expressions.

## GitHub
- Header: `X-GitHub-Event`
- Signature: `X-Hub-Signature-256` (HMAC SHA-256). `X-Hub-Signature` (HMAC SHA-1) is accepted for GitHub Enterprise Server.
- Path: `/webhooks/github`

## GitLab
- Header: `X-Gitlab-Event`
- Secret: `X-Gitlab-Token` (optional)
- Path: `/webhooks/gitlab`

## Bitbucket (Cloud)
- Header: `X-Event-Key`
- Secret: `X-Hook-UUID` (optional)
- Path: `/webhooks/bitbucket`

## Compatibility Notes
- GitHub payloads use `pull_request` (singular), not `pull_requests`.
- Bitbucket events use keys like `pullrequest:created`.
- GitLab event names come from `X-Gitlab-Event` (e.g., `Merge Request Hook`).

For rule syntax and JSONPath examples, see `docs/rules.md`.

## Message Metadata

Published messages include metadata keys that workers can use:
- `installation_id`: Provider installation ID.
- `provider_instance_key`: Provider instance hash (used to resolve enterprise vs. cloud).
- `request_id`: Request trace ID.
- `log_id`: Event log record ID.
- `driver`: Broker driver name (amqp/nats/kafka).

## Event Logs

Event log records persist the webhook headers and raw body, plus a `body_hash`
for duplicate detection. The EventLogs API returns these fields to workers
and clients.

## Debugging
Check logs for:
- `event provider=... name=... topics=[...]`
- `rule debug: when=... params=...`
