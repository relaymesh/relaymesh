import { SCMClientsClient } from "./api.js";
import { MetadataKeyInstallationID, MetadataKeyProviderInstanceKey } from "./metadata.js";
import { newProviderClient, GitHubClientFromEvent, GitLabClientFromEvent, BitbucketClientFromEvent } from "./scm_clients.js";
import { resolveOAuth2Config } from "./oauth2.js";
import type { APIClientOptions } from "./api.js";
import type { ClientProvider } from "./client.js";
import type { Event } from "./event.js";
import type { WorkerContext } from "./context.js";
import type { OAuth2Config } from "./oauth2.js";

const defaultCacheSize = 10;
const defaultCacheSkewMs = 30000;

export interface RemoteSCMClientProviderOptions {
  endpoint?: string;
  apiKey?: string;
  oauth2Config?: OAuth2Config;
  cacheSize?: number;
  cacheSkewMs?: number;
}

export class RemoteSCMClientProvider implements ClientProvider {
  private endpoint: string;
  private apiKey: string;
  private oauth2Config?: OAuth2Config;
  private cache: SCMClientCache;
  private cacheSkewMs: number;

  constructor(opts: RemoteSCMClientProviderOptions = {}) {
    this.endpoint = resolveEndpoint(opts.endpoint);
    this.apiKey = (opts.apiKey ?? "").trim();
    this.oauth2Config = resolveOAuth2Config(opts.oauth2Config);
    const skew = opts.cacheSkewMs ?? defaultCacheSkewMs;
    this.cacheSkewMs = skew < 0 ? 0 : skew;
    this.cache = new SCMClientCache(opts.cacheSize ?? defaultCacheSize);
  }

  bindAPIClient(opts: APIClientOptions): void {
    if (opts.baseUrl) {
      this.endpoint = resolveEndpoint(opts.baseUrl);
    }
    if (opts.apiKey) {
      this.apiKey = opts.apiKey;
    }
    if (opts.oauth2Config) {
      this.oauth2Config = opts.oauth2Config;
    }
  }

  BindAPIClient(opts: APIClientOptions): void {
    this.bindAPIClient(opts);
  }

  async client(ctx: WorkerContext, evt: Event): Promise<unknown> {
    return this.Client(ctx, evt);
  }

  async Client(ctx: WorkerContext, evt: Event): Promise<unknown> {
    if (!evt) {
      throw new Error("event is required");
    }
    const provider = (evt.provider ?? "").trim();
    if (!provider) {
      throw new Error("provider is required");
    }
    const installationId = (evt.metadata?.[MetadataKeyInstallationID] ?? "").trim();
    if (!installationId) {
      throw new Error("installation_id missing from metadata");
    }
    const instanceKey = (evt.metadata?.[MetadataKeyProviderInstanceKey] ?? "").trim();
    const cacheKey = [provider, installationId, instanceKey].join("|");
    const cached = this.cache.get(cacheKey, this.cacheSkewMs);
    if (cached) {
      return cached;
    }

    const client = new SCMClientsClient({
      baseUrl: this.endpoint,
      apiKey: this.apiKey,
      oauth2Config: this.oauth2Config,
      tenantId: ctx.tenantId,
    });
    const record = await client.getSCMClient(provider, installationId, instanceKey, ctx);
    if (!record.accessToken) {
      throw new Error("scm access token missing");
    }
    const created = newProviderClient(record.provider, record.accessToken, record.apiBaseUrl);
    this.cache.set(cacheKey, created, record.expiresAt);
    return created;
  }
}

export function NewRemoteSCMClientProvider(opts: RemoteSCMClientProviderOptions = {}): RemoteSCMClientProvider {
  return new RemoteSCMClientProvider(opts);
}

export function GitHubClient(evt: { client?: unknown }) {
  return GitHubClientFromEvent(evt);
}

export function GitLabClient(evt: { client?: unknown }) {
  return GitLabClientFromEvent(evt);
}

export function BitbucketClient(evt: { client?: unknown }) {
  return BitbucketClientFromEvent(evt);
}

class SCMClientCache {
  private readonly store = new Map<string, CacheEntry>();
  constructor(private readonly maxSize: number) {}

  get(key: string, skewMs: number): unknown | undefined {
    const entry = this.store.get(key);
    if (!entry) {
      return undefined;
    }
    if (entry.expiresAt && entry.expiresAt.getTime() - skewMs <= Date.now()) {
      this.store.delete(key);
      return undefined;
    }
    this.store.delete(key);
    this.store.set(key, entry);
    return entry.client;
  }

  set(key: string, client: unknown, expiresAt?: Date): void {
    if (!client) {
      return;
    }
    if (this.store.has(key)) {
      this.store.delete(key);
    }
    this.store.set(key, { client, expiresAt });
    if (this.store.size <= Math.max(1, this.maxSize)) {
      return;
    }
    const oldestKey = this.store.keys().next().value;
    if (oldestKey) {
      this.store.delete(oldestKey);
    }
  }
}

interface CacheEntry {
  client: unknown;
  expiresAt?: Date;
}

function resolveEndpoint(explicit?: string): string {
  const trimmed = (explicit ?? "").trim();
  if (trimmed) {
    return trimmed.replace(/\/+$/, "");
  }
  const envEndpoint = envValue("RELAYMESH_ENDPOINT");
  if (envEndpoint) {
    return envEndpoint;
  }
  const envBase = envValue("RELAYMESH_API_BASE_URL");
  if (envBase) {
    return envBase;
  }
  return "http://localhost:8080";
}

function envValue(key: string): string {
  return (process.env[key] ?? "").trim();
}
