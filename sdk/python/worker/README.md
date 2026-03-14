# relaymesh Python Worker SDK

The Python worker SDK mirrors the Go/TypeScript worker interfaces and connects to the relaymesh control plane for rules, drivers, event logs, and SCM credentials.

## Install

```bash
pip install relaymesh
```

No additional packages are required.

## Quick Start

```python
import signal
import threading

from relaymesh.listener import Listener
from relaymesh.scm_client_provider import NewRemoteSCMClientProvider
from relaymesh.worker import (
    New,
    WithClientProvider,
    WithConcurrency,
    WithEndpoint,
    WithListener,
)

stop = threading.Event()

def shutdown(_signum, _frame):
    stop.set()

signal.signal(signal.SIGINT, shutdown)
signal.signal(signal.SIGTERM, shutdown)

wk = New(
    WithEndpoint("http://localhost:8080"),
    WithClientProvider(NewRemoteSCMClientProvider()),
    WithConcurrency(4),
    WithListener(Listener()),
)

def handle(ctx, evt):
    installation_id = evt.metadata.get("installation_id", "")
    print(f"topic={evt.topic} provider={evt.provider} type={evt.type} installation={installation_id}")
    if evt.payload:
        print(f"payload bytes={len(evt.payload)}")

wk.HandleRule("85101e9f-3bcf-4ed0-b561-750c270ef6c3", handle)

wk.Run(stop)
```

## Handler Signature

Handlers can accept either `(event)` or `(ctx, event)`:

```python
def handle(evt):
    print(evt.provider, evt.type)

def handle_with_ctx(ctx, evt):
    print(ctx.request_id, evt.topic)
```

## Using SCM Clients (GitHub/GitLab/Bitbucket)

```python
from relaymesh.scm_client_provider import BitbucketClient, GitHubClient, GitLabClient

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
        user = client.request_json("GET", "/user")
        print(user)
```

## Retry, concurrency, and status updates

- Set retry behavior with `WithRetryCount(n)`.
- Set in-flight processing with `WithConcurrency(n)`.
- Use `WithListener(...)` to log lifecycle callbacks and status outcomes.
- Worker status updates are automatic: success on clean return, failed on raised exception.

## OAuth2 mode

Use `mode="client_credentials"` for worker OAuth2 token flow.

```python
from relaymesh.oauth2 import OAuth2Config
from relaymesh.worker import WithOAuth2Config

wk = New(
    WithOAuth2Config(
        OAuth2Config(
            enabled=True,
            mode="client_credentials",
            client_id="your-client-id",
            client_secret="your-client-secret",
            token_url="https://issuer.example.com/oauth/token",
            scopes=["githook.read", "githook.write"],
        )
    )
)
```

## Environment Variables

The worker will read defaults from:

- `GITHOOK_ENDPOINT` or `GITHOOK_API_BASE_URL`
- `GITHOOK_API_KEY`
- `GITHOOK_TENANT_ID`
