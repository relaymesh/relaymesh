# sdk/ — Worker SDKs (Go / TypeScript / Python)

## OVERVIEW

Three language SDKs that consume webhook events from message brokers. Go is the reference implementation; TypeScript and Python maintain API parity.

## SHARED CONTRACT

All SDKs implement the same pattern:
1. `New(opts...)` → create worker with functional options
2. `HandleRule("rule-id", handler)` → register handler for a server-stored rule
3. `Run(ctx)` → fetch rule → resolve driver → subscribe to broker topic → dispatch events

**Events flow:** Server publishes `EventPayload` (protobuf) to broker → SDK decodes → dispatches to handler with unified `Event` struct.

## STRUCTURE

```
sdk/
├── go/worker/           # 34 files — reference implementation
│   ├── worker.go        # Worker type: HandleRule, HandleType, Run, Close
│   ├── options.go       # WithEndpoint, WithAPIKey, WithConcurrency, WithSubscriber, etc.
│   ├── event.go         # Event: Provider, Type, Topic, Metadata, Payload, Client
│   ├── handler.go       # Handler = func(ctx, *Event) error; Middleware = func(Handler) Handler
│   ├── subscriber.go    # Subscriber interface + AMQP/NATS/Kafka via Relaybus
│   ├── codec.go         # Codec interface — protobuf EventPayload → Event
│   ├── client.go        # ClientProvider interface — SCM client injection
│   ├── scm_client_*.go  # RemoteSCMClientProvider (LRU-cached server-side credentials)
│   ├── *_client.go      # API clients: Rules, Drivers, EventLogs, Installations, SCM
│   ├── listener.go      # Lifecycle hooks: OnStart, OnExit, OnMessageStart, OnError
│   ├── retry.go         # RetryPolicy interface
│   └── *_test.go        # Tests (standard testing package)
│
├── typescript/worker/   # 19 src files — mirrors Go API
│   ├── src/index.ts     # Exports: New, With*, HandleRule, Run
│   ├── src/worker.ts    # Worker class
│   ├── src/event.ts     # Event type
│   ├── src/subscriber.ts
│   ├── src/codec.ts
│   └── package.json     # @relaymesh/sdk on npm
│
└── python/worker/       # 17 src files — mirrors Go API
    ├── relaymesh/       # primary package name
    │   ├── __init__.py  # Exports: New, With*, HandleRule, Run
    │   ├── worker.py    # Worker class
    │   ├── event.py     # Event dataclass
    │   ├── subscriber.py
    │   └── codec.py
└── setup.py         # relaymesh on PyPI
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| Change worker lifecycle | `{lang}/worker.go` / `worker.ts` / `worker.py` | Core Run loop + shutdown |
| Add configuration option | `go/worker/options.go` first | Then port to TS (`index.ts`) and Python (`__init__.py`) |
| Change event decoding | `*/codec.*` | Protobuf → Event conversion |
| Change broker subscription | `*/subscriber.*` | Relaybus adapter selection |
| Add SCM client helper | `go/worker/scm_clients.go` | Type assertion helpers: `GitHubClient()`, `GitLabClient()`, `BitbucketClient()` |
| Change API communication | `go/worker/*_client.go` | ConnectRPC clients for server APIs |
| Add middleware | `go/worker/handler.go` | `Middleware = func(Handler) Handler` |

## KEY INTERFACES (GO — reference for all SDKs)

```go
// Core processing
type Handler    func(ctx context.Context, evt *Event) error
type Middleware func(Handler) Handler

// Pluggable components
type Subscriber     interface { Start(ctx, topic, handler); Close() }
type Codec          interface { Decode(msg) (*Event, error) }
type ClientProvider interface { Client(ctx, evt) (interface{}, error) }
type RetryPolicy    interface { OnError(ctx, evt, err) bool }
type Logger         interface { Printf(format, args...) }
```

## CONVENTIONS

- **Go is reference**: Always implement in Go first, then port to TS/Python
- **Functional options**: All config via `With*()` — no constructor args
- **Server-side credentials**: Workers NEVER store SCM tokens. `RemoteSCMClientProvider` fetches from server with LRU cache (10 entries, 30s expiry skew)
- **Rule-based subscription**: `HandleRule(id)` → API call to fetch rule → resolve driver → subscribe to emit topic on that driver's broker
- **Protobuf wire format**: `EventPayload` message defined in `githooks.proto`, decoded by `Codec`
- **Event metadata keys**: `log_id`, `provider`, `event`, `topic`, `driver`, `installation_id`, `provider_instance_key`, `request_id`
- **Status tracking**: Workers call `UpdateEventLogStatus(log_id, status)` — statuses: `delivered`, `success`, `failed`

## ANTI-PATTERNS

- **Never store credentials in workers** — always fetch via `GetSCMClient` API
- **Never break parity** — changes to one SDK should be reflected in all three
- **Never add broker-specific code** — use Relaybus adapters (`relaybus-amqp`, `relaybus-kafka`, `relaybus-nats`)
- **TS/Python have no unit tests** — tested via `examples/`. Add tests if making significant changes

## AUTH MODES

Workers authenticate to the server API via:
- **API Key**: `WithAPIKey("key")` → `x-api-key` header
- **OAuth2**: `WithOAuth2Config({...})` → OIDC client credentials → Bearer token
- **Tenant**: `WithTenant("id")` → `X-Tenant-ID` header
