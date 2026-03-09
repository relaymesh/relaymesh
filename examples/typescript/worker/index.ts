import {
  New,
  WithEndpoint,
  WithClientProvider,
  WithAPIKey,
  WithTenant,
  WithConcurrency,
  WithRetryCount,
  WithLogger,
  WithListener,
  NewRemoteSCMClientProvider,
  GitHubClient,
  GitLabClient,
  BitbucketClient,
} from "@relaymesh/githook";

async function main() {
  const endpoint = process.env.GITHOOK_ENDPOINT ?? "https://relaymesh.vercel.app/api/connect";
  const ruleId = process.env.GITHOOK_RULE_ID ?? "85101e9f-3bcf-4ed0-b561-750c270ef6c3";
  const apiKey = process.env.GITHOOK_API_KEY ?? "";
  const tenantId = process.env.GITHOOK_TENANT_ID ?? "";
  const concurrency = intFromEnv(process.env.GITHOOK_CONCURRENCY, 4);
  const retryCount = intFromEnv(process.env.GITHOOK_RETRY_COUNT, 1);
  console.log(
    `config endpoint=${endpoint} apiKeySet=${apiKey.length > 0} tenantId=${tenantId || "(empty)"} concurrency=${concurrency} retryCount=${retryCount}`,
  );

  const provider = NewRemoteSCMClientProvider();

  const options = [
    WithEndpoint(endpoint),
    WithClientProvider(provider),
    WithConcurrency(concurrency),
    WithRetryCount(retryCount),
    WithLogger({
      printf: (format, ...args) => {
        const rendered = format.replace(/%s/g, () => String(args.shift() ?? ""));
        console.log(`example-worker ${rendered}`);
      },
    }),
  ];
  options.push(
    WithListener({
      onMessageStart: (_ctx, evt) => {
        console.log(`listener start log_id=${evt.metadata?.log_id ?? ""} topic=${evt.topic}`);
      },
      onMessageFinish: (_ctx, evt, err) => {
        const status = err ? "failed" : "success";
        console.log(
          `listener finish log_id=${evt.metadata?.log_id ?? ""} status=${status} err=${err?.message ?? ""}`,
        );
      },
      onError: (_ctx, evt, err) => {
        console.log(
          `listener error log_id=${evt?.metadata?.log_id ?? ""} err=${err.message}`,
        );
      },
    }),
  );
  if (apiKey) {
    options.push(WithAPIKey(apiKey));
  }
  if (tenantId) {
    options.push(WithTenant(tenantId));
  }
  const wk = New(...options);

  wk.HandleRule(ruleId, async (_ctx, evt) => {
    if (!evt) {
      return;
    }

    const providerName = evt.provider?.toLowerCase?.() ?? "";
    console.log(`handler topic=${evt.topic} provider=${providerName} type=${evt.type}`);

    if (providerName === "github") {
      const gh = GitHubClient(evt);
      if (!gh) {
        console.log("github client not available (installation may not be configured)");
        return;
      }

      const { owner, repo } = repositoryFromEvent(evt);
      if (!owner || !repo) {
        console.log("repository info missing in payload; skipping github read");
        return;
      }

      try {
        const commits = await gh.requestJSON<Record<string, unknown>[]>("GET", `/repos/${owner}/${repo}/commits?per_page=5`);
        console.log(`github commits count=${commits.length}`);
        for (let i = 0; i < commits.length; i++) {
          const sha = String(commits[i].sha ?? "").slice(0, 7);
          const msg = firstLine(String((commits[i].commit as Record<string, unknown>)?.message ?? ""));
          console.log(`  commit[${i + 1}] sha=${sha} message=${msg}`);
        }
      } catch (err) {
        console.log(`github list commits failed owner=${owner} repo=${repo} err=${String(err)}`);
      }
      return;
    }

    if (providerName === "gitlab") {
      const gl = GitLabClient(evt);
      if (!gl) {
        console.log("gitlab client not available (installation may not be configured)");
        return;
      }

      const { owner: glOwner, repo: glRepo } = repositoryFromEvent(evt);
      if (!glOwner || !glRepo) {
        console.log("repository info missing in payload; skipping gitlab read");
        return;
      }

      try {
        const project = encodeURIComponent(`${glOwner}/${glRepo}`);
        const commits = await gl.requestJSON<Record<string, unknown>[]>("GET", `/projects/${project}/repository/commits?per_page=5`);
        console.log(`gitlab commits count=${commits.length}`);
        for (let i = 0; i < commits.length; i++) {
          const sha = String(commits[i].short_id ?? "");
          const msg = firstLine(String(commits[i].title ?? ""));
          console.log(`  commit[${i + 1}] sha=${sha} message=${msg}`);
        }
      } catch (err) {
        console.log(`gitlab list commits failed err=${String(err)}`);
      }
      return;
    }

    if (providerName === "bitbucket") {
      const bb = BitbucketClient(evt);
      if (!bb) {
        console.log("bitbucket client not available (installation may not be configured)");
        return;
      }

      const { owner: bbOwner, repo: bbRepo } = repositoryFromEvent(evt);
      if (!bbOwner || !bbRepo) {
        console.log("repository info missing in payload; skipping bitbucket read");
        return;
      }

      try {
        const result = await bb.requestJSON<Record<string, unknown>>("GET", `/repositories/${bbOwner}/${bbRepo}/commits?pagelen=5`);
        const values = Array.isArray(result.values) ? result.values as Record<string, unknown>[] : [];
        console.log(`bitbucket commits count=${values.length}`);
        for (let i = 0; i < values.length; i++) {
          const sha = String(values[i].hash ?? "").slice(0, 7);
          const msg = firstLine(String(values[i].message ?? ""));
          console.log(`  commit[${i + 1}] sha=${sha} message=${msg}`);
        }
      } catch (err) {
        console.log(`bitbucket list commits failed err=${String(err)}`);
      }
      return;
    }

    console.log(`unsupported provider=${providerName}; skipping scm call`);
  });

  await wk.Run();
}

function firstLine(s: string): string {
  const idx = s.indexOf("\n");
  return idx >= 0 ? s.slice(0, idx).trim() : s.trim();
}

function intFromEnv(raw: string | undefined, fallback: number): number {
  if (!raw) {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function repositoryFromEvent(evt: { normalized?: Record<string, unknown> }): {
  owner: string;
  repo: string;
} {
  const repoValue = evt.normalized?.["repository"];
  if (!repoValue || typeof repoValue !== "object") {
    return { owner: "", repo: "" };
  }

  const repoMap = repoValue as Record<string, unknown>;
  const fullName = repoMap["full_name"];
  if (typeof fullName === "string") {
    const parts = fullName.trim().split("/", 2);
    if (parts.length === 2 && parts[0] && parts[1]) {
      return { owner: parts[0], repo: parts[1] };
    }
  }

  const name = typeof repoMap["name"] === "string" ? repoMap["name"] : "";
  const ownerMap = repoMap["owner"];
  const ownerLogin =
    ownerMap && typeof ownerMap === "object"
      ? (ownerMap as Record<string, unknown>)["login"]
      : "";
  return {
    owner: typeof ownerLogin === "string" ? ownerLogin.trim() : "",
    repo: typeof name === "string" ? name.trim() : "",
  };
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
