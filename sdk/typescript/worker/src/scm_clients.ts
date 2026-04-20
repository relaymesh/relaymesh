export interface SCMClient {
  request(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<Response>;
  requestJSON<T = unknown>(
    method: string,
    path: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<T>;
}

export class GitHubClient implements SCMClient {
  constructor(private readonly token: string, private readonly baseUrl: string) {}

  async request(
    method: string,
    path: string,
    body?: unknown,
    headers: Record<string, string> = {},
  ): Promise<Response> {
    const url = resolveURL(this.baseUrl, path);
    return fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "application/vnd.github+json",
        "Content-Type": "application/json",
        ...headers,
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async requestJSON<T>(
    method: string,
    path: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<T> {
    const resp = await this.request(method, path, body, headers);
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`github request failed (${resp.status}): ${text}`);
    }
    return resp.json() as Promise<T>;
  }
}

export class GitLabClient implements SCMClient {
  constructor(private readonly token: string, private readonly baseUrl: string) {}

  async request(
    method: string,
    path: string,
    body?: unknown,
    headers: Record<string, string> = {},
  ): Promise<Response> {
    const url = resolveURL(this.baseUrl, path);
    return fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
        ...headers,
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async requestJSON<T>(
    method: string,
    path: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<T> {
    const resp = await this.request(method, path, body, headers);
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`gitlab request failed (${resp.status}): ${text}`);
    }
    return resp.json() as Promise<T>;
  }
}

export class BitbucketClient implements SCMClient {
  constructor(private readonly token: string, private readonly baseUrl: string) {}

  async request(
    method: string,
    path: string,
    body?: unknown,
    headers: Record<string, string> = {},
  ): Promise<Response> {
    const url = resolveURL(this.baseUrl, path);
    return fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
        ...headers,
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async requestJSON<T>(
    method: string,
    path: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<T> {
    const resp = await this.request(method, path, body, headers);
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`bitbucket request failed (${resp.status}): ${text}`);
    }
    return resp.json() as Promise<T>;
  }
}

export class SlackClient implements SCMClient {
  constructor(private readonly token: string, private readonly baseUrl: string) {}

  async request(
    method: string,
    path: string,
    body?: unknown,
    headers: Record<string, string> = {},
  ): Promise<Response> {
    const url = resolveURL(this.baseUrl, path);
    return fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
        ...headers,
      },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  async requestJSON<T>(
    method: string,
    path: string,
    body?: unknown,
    headers?: Record<string, string>,
  ): Promise<T> {
    const resp = await this.request(method, path, body, headers);
    if (!resp.ok) {
      const text = await resp.text().catch(() => "");
      throw new Error(`slack request failed (${resp.status}): ${text}`);
    }
    return resp.json() as Promise<T>;
  }
}

export function GitHubClientFromEvent(evt: { client?: unknown }): GitHubClient | undefined {
  return evt?.client instanceof GitHubClient ? evt.client : undefined;
}

export function GitLabClientFromEvent(evt: { client?: unknown }): GitLabClient | undefined {
  return evt?.client instanceof GitLabClient ? evt.client : undefined;
}

export function BitbucketClientFromEvent(evt: { client?: unknown }): BitbucketClient | undefined {
  return evt?.client instanceof BitbucketClient ? evt.client : undefined;
}

export function SlackClientFromEvent(evt: { client?: unknown }): SlackClient | undefined {
  return evt?.client instanceof SlackClient ? evt.client : undefined;
}

export function newProviderClient(provider: string, token: string, baseUrl: string): SCMClient {
  const normalized = (provider ?? "").trim().toLowerCase();
  switch (normalized) {
    case "github":
      return new GitHubClient(token, resolveAPIBase(baseUrl, normalized));
    case "gitlab":
      return new GitLabClient(token, resolveAPIBase(baseUrl, normalized));
    case "bitbucket":
      return new BitbucketClient(token, resolveAPIBase(baseUrl, normalized));
    case "slack":
      return new SlackClient(token, resolveAPIBase(baseUrl, normalized));
    default:
      throw new Error(`unsupported provider for scm client: ${provider}`);
  }
}

export function NewProviderClient(provider: string, token: string, baseUrl: string): SCMClient {
  return newProviderClient(provider, token, baseUrl);
}

function resolveAPIBase(baseUrl: string, provider: string): string {
  const trimmed = (baseUrl ?? "").trim();
  if (trimmed) {
    return trimmed;
  }
  switch ((provider ?? "").toLowerCase()) {
    case "github":
      return "https://api.github.com";
    case "gitlab":
      return "https://gitlab.com/api/v4";
    case "bitbucket":
      return "https://api.bitbucket.org/2.0";
    case "slack":
      return "https://slack.com/api";
    default:
      return trimmed;
  }
}

function resolveURL(baseUrl: string, path: string): string {
  if (!path) {
    return baseUrl;
  }
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }
  const normalizedBase = (baseUrl ?? "").replace(/\/+$/, "");
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return `${normalizedBase}${normalizedPath}`;
}
