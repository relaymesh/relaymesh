# Observability

relaymesh exposes lightweight observability signals that work with minimal setup.

## Request IDs

Incoming requests use or generate `X-Request-Id`. The server echoes it back in responses and includes it in logs and published message metadata.

## Logs to look for

The server logs include event routing decisions and rule evaluation details. Common entries:

- `event provider=... name=... topics=[...]`
- `rule debug: when=... params=...`

Use the request ID to correlate webhook receipt, rule evaluation, and publish steps.

## Message metadata

Published messages include metadata fields that help with tracing:

- `request_id` for end-to-end correlation
- `log_id` for event log lookup
- `provider_instance_key` for cloud vs. enterprise resolution

## Debug checklist

If a webhook does not appear in your worker:

1. Confirm the webhook target URL matches `/webhooks/<provider>`.
2. Check the server log for the request ID and event entry.
3. Verify a rule matches the payload (`rules match` can test locally).
4. Confirm the driver config is enabled and reachable.
5. Check event logs for `matched=false` or `status=failed` entries.
