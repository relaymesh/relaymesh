# SDK Client Injection

Workers can attach SCM clients (GitHub/GitLab/Bitbucket) to each event. The default approach is to ask the server for credentials and build clients locally with a small LRU cache.

## When to use each approach

- Remote SCM clients: best for most deployments, because the server resolves cloud vs. enterprise and handles credentials.
- Custom clients: useful when you need to route through a proxy or reuse existing auth infrastructure.

## Remote SCM clients (recommended)

The worker uses the event metadata (installation + provider instance key) to request credentials from the server. The server decides cloud vs. enterprise and returns the correct auth details.

Go:

```go
wk := worker.New(
  worker.WithEndpoint("http://localhost:8080"),
  worker.WithClientProvider(worker.NewRemoteSCMClientProvider()),
)

wk.HandleRule("<rule-id>", func(ctx context.Context, evt *worker.Event) error {
  switch strings.ToLower(evt.Provider) {
  case "github":
    if gh, ok := worker.GitHubClient(evt); ok {
      _, _, _ = gh.Repositories.List(ctx, "", nil)
    }
  case "gitlab":
    if _, ok := worker.GitLabClient(evt); ok {
      // Use gitlab client APIs here.
    }
  case "bitbucket":
    if _, ok := worker.BitbucketClient(evt); ok {
      // Use bitbucket client APIs here.
    }
  }
  return nil
})
```

TypeScript:

```ts
import {
  New,
  WithEndpoint,
  WithClientProvider,
  NewRemoteSCMClientProvider,
  GitHubClient,
  GitLabClient,
  BitbucketClient,
} from "@relaymesh/sdk";

const worker = New(
  WithEndpoint("http://localhost:8080"),
  WithClientProvider(NewRemoteSCMClientProvider()),
);

worker.HandleRule("<rule-id>", async (_ctx, evt) => {
  switch ((evt.provider || "").toLowerCase()) {
    case "github": {
      const gh = GitHubClient(evt);
      if (gh) await gh.requestJSON("GET", "/user");
      break;
    }
    case "gitlab": {
      const gl = GitLabClient(evt);
      if (gl) await gl.requestJSON("GET", "/user");
      break;
    }
    case "bitbucket": {
      const bb = BitbucketClient(evt);
      if (bb) await bb.requestJSON("GET", "/user");
      break;
    }
  }
});
```

If a client cannot be resolved, the helpers return `nil`/`undefined`. Treat that as a non-fatal condition and continue handling the event.

Python:

```python
from relaymesh import (
    New,
    WithEndpoint,
    WithClientProvider,
    NewRemoteSCMClientProvider,
    GitHubClient,
    GitLabClient,
    BitbucketClient,
)

wk = New(
    WithEndpoint("http://localhost:8080"),
    WithClientProvider(NewRemoteSCMClientProvider()),
)

def handler(ctx, evt):
    provider = (evt.provider or "").lower()
    if provider == "github":
        client = GitHubClient(evt)
    elif provider == "gitlab":
        client = GitLabClient(evt)
    elif provider == "bitbucket":
        client = BitbucketClient(evt)
    else:
        client = None

    if client:
        client.request_json("GET", "/user")

wk.HandleRule("<rule-id>", handler)
```

## Custom client injection

If you want full control, inject your own client resolver.

Go:

```go
wk := worker.New(
  worker.WithClientProvider(worker.ClientProviderFunc(func(ctx context.Context, evt *worker.Event) (interface{}, error) {
    return newSCMProxyClient(os.Getenv("SCM_PROXY_URL")), nil
  })),
)
```

## Event log status updates

Each SDK updates event log status automatically after your handler runs:

- Successful return/completion updates status to `success`.
- Returned errors (Go) or thrown/raised exceptions (TypeScript/Python) update status to `failed` and include the error message.

## Runtime options (retry, concurrency, logging)

All three SDKs support equivalent runtime controls:

- Retry count: `WithRetryCount(n)` retries handler execution up to `n` extra times (total attempts = `n + 1`).
- Concurrency: `WithConcurrency(n)` limits in-flight message handling.
- Logging hooks: `WithListener(...)` gives lifecycle callbacks; `WithLogger(...)` overrides default logger output.
