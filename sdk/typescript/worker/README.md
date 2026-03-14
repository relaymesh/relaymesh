# TypeScript Worker SDK

This SDK consumes githook events from Relaybus and decodes the protobuf
`EventPayload` into a usable `Event` object.

Supported drivers: amqp, kafka, nats.

## Install

```sh
npm install @relaymesh/sdk
```

No additional packages are required.

## relaymesh Worker Example (rule-id only)

```ts
import * as worker from "@relaymesh/sdk";

async function main() {
  const wk = worker.New(
    worker.WithEndpoint("http://localhost:8080"),
  );
  wk.HandleRule("rule-id", (ctx, event) => {
    console.log(ctx.tenantId, event.provider, event.type, event.topic);
    console.log(event.payload.toString("utf8"));
  });

  await wk.Run();
}

main().catch(console.error);
```

## Go-style example

```ts
import * as worker from "@relaymesh/sdk";

const controller = new AbortController();
process.on("SIGINT", () => controller.abort());
process.on("SIGTERM", () => controller.abort());

const wk = worker.New(
  worker.WithEndpoint("https://githook-app.vercel.app/api/connect"),
);

wk.HandleRule("85101e9f-3bcf-4ed0-b561-750c270ef6c3", (ctx, evt) => {
  if (!evt) {
    return;
  }
  console.log(
    `topic=${evt.topic} provider=${evt.provider} type=${evt.type} installation=${evt.metadata["installation_id"]}`,
  );
  if (evt.payload.length > 0) {
    console.log(`payload bytes=${evt.payload.length}`);
  }
});

await wk.Run(controller.signal);
```

## API-driven drivers

Rule handlers are the recommended way to subscribe. If you need to bind handlers
to explicit topics or driver IDs, you can still do so.

```ts
import * as worker from "@relaymesh/sdk";

async function main() {
  const subscriber = worker.buildSubscriber({
    driver: "amqp",
    amqp: { url: "amqp://guest:guest@localhost:5672/" },
  });

  const wk = worker.New(
    worker.WithSubscriber(subscriber),
    worker.WithEndpoint("http://localhost:8080"),
    worker.WithAPIKey(process.env.GITHOOK_API_KEY ?? ""),
    worker.WithTopics("pr.opened.ready"),
  );

  wk.HandleTopic("pr.opened.ready", "driver-id", (ctx, event) => {
    console.log(ctx.requestId, event.topic);
  });

  await wk.Run();
}

main().catch(console.error);
```

## Rule handlers

```ts
const wk = worker.New();

wk.HandleRule("rule-id", (ctx, event) => {
  console.log(event.topic, event.provider);
});
```

## OAuth2 for API calls

If you use OAuth2 instead of API keys for control-plane calls:

```ts
const wk = worker.New(
  worker.WithEndpoint("http://localhost:8080"),
  worker.WithOAuth2Config({
    enabled: true,
    mode: "client_credentials",
    clientId: process.env.GITHOOK_OAUTH2_CLIENT_ID ?? "",
    clientSecret: process.env.GITHOOK_OAUTH2_CLIENT_SECRET ?? "",
    tokenUrl: process.env.GITHOOK_OAUTH2_TOKEN_URL ?? "",
    scopes: ["githook.read", "githook.write"],
  }),
);
```

## Server-resolved SCM clients

```ts
const wk = worker.New(
  worker.WithEndpoint("http://localhost:8080"),
  worker.WithClientProvider(worker.NewRemoteSCMClientProvider()),
  worker.WithConcurrency(4),
  worker.WithRetryCount(1),
  worker.WithListener({
    onMessageFinish: (_ctx, evt, err) => {
      console.log(`log_id=${evt.metadata.log_id ?? ""} status=${err ? "failed" : "success"}`);
    },
  }),
);

wk.HandleRule("rule-id", async (_ctx, evt) => {
  switch ((evt.provider || "").toLowerCase()) {
    case "github": {
      const gh = worker.GitHubClient(evt);
      if (gh) await gh.requestJSON("GET", "/user");
      break;
    }
    case "gitlab": {
      const gl = worker.GitLabClient(evt);
      if (gl) await gl.requestJSON("GET", "/user");
      break;
    }
    case "bitbucket": {
      const bb = worker.BitbucketClient(evt);
      if (bb) await bb.requestJSON("GET", "/user");
      break;
    }
  }
});
```
