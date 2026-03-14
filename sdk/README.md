# SDKs

This directory groups language-specific SDKs that build on top of the relaymesh event model and a shared, language-agnostic contract.

- `go/`: Go worker SDK (reference implementation).
- `typescript/worker`: TypeScript worker SDK (Relaybus JS adapters, parity with Go worker APIs).
- `python/worker`: Python worker SDK (Relaybus adapters, parity with Go/TypeScript worker APIs).

Planned:
- `rust/`: Thin client bindings that consume the same event envelope and rules DSL.

## Shared Contract

All SDKs are expected to:

- Consume the same event envelope (`provider`, `topic`, `payload`, `metadata`).
- Rely on the same rules DSL defined and stored by the server.
- Keep provider credentials server-side and let the worker fetch short-lived SCM credentials from the server.
- Support SCM clients for GitHub, GitLab, and Bitbucket.
- Auto-update event log status (`success`/`failed`) based on handler outcome.
- Expose runtime controls for retry count, concurrency, listeners, and logger injection.

## Go Worker Quick Start

Use the API-driven worker so you only need the endpoint and API key:

```go
package main

import (
  "context"
  "os"

  "githook/sdk/go/worker"
)

func main() {
  wk := worker.New(
    worker.WithEndpoint(os.Getenv("GITHOOK_ENDPOINT")),
    worker.WithAPIKey(os.Getenv("GITHOOK_API_KEY")),
  )

  wk.HandleRule("rule-id", func(ctx context.Context, evt *worker.Event) error {
    return nil
  })

  _ = wk.Run(context.Background())
}
```

## Language Support Note

Relaybus provides Go, JavaScript, and Python adapters for amqp/kafka/nats, so the
Go, TypeScript, and Python worker SDKs are supported. Other languages would need their own
broker consumers and runtime logic. We plan to build Relaybus‑like runtimes for
additional languages, but that work is still in the pipeline.
