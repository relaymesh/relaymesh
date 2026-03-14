import { DefaultCodec } from "./codec.js";
import { DriversClient, EventLogsClient, RulesClient } from "./api.js";
import { subscriberConfigFromDriver } from "./driver_config.js";
import { EventLogStatusFailed, EventLogStatusSuccess } from "./event_log_status.js";
import { MetadataKeyDriver, MetadataKeyLogID, MetadataKeyRequestID, MetadataKeyTenantID } from "./metadata.js";
import { NoRetry, normalizeRetryDecision } from "./retry.js";
import { buildSubscriber } from "./subscriber.js";
import type { APIClientOptions } from "./api.js";
import type { ClientProvider } from "./client.js";
import type { Codec } from "./codec.js";
import type { WorkerContext } from "./context.js";
import type { Event } from "./event.js";
import type { Listener } from "./listener.js";
import { resolveOAuth2Config } from "./oauth2.js";
import type { OAuth2Config } from "./oauth2.js";
import type { RetryPolicy } from "./retry.js";
import type { Subscriber } from "./subscriber.js";
import type { RelaybusMessage } from "./types.js";

export type Handler =
  | ((ctx: WorkerContext, event: Event) => Promise<void> | void)
  | ((event: Event) => Promise<void> | void);

export type ContextualHandler = (ctx: WorkerContext, event: Event) => Promise<void> | void;

export type Middleware = (next: ContextualHandler) => ContextualHandler;

export interface Logger {
  printf?: (format: string, ...args: unknown[]) => void;
  Printf?: (format: string, ...args: unknown[]) => void;
}

export interface WorkerOptions {
  subscriber?: Subscriber;
  topics?: string[];
  codec?: Codec;
  logger?: Logger;
  concurrency?: number;
  middleware?: Middleware[];
  retry?: RetryPolicy;
  retryCount?: number;
  listeners?: Listener[];
  clientProvider?: ClientProvider;
  endpoint?: string;
  apiKey?: string;
  oauth2Config?: OAuth2Config;
  tenantId?: string;
  defaultDriverId?: string;
  validateTopics?: boolean;
}

export type WorkerOption = (worker: Worker) => void;

export class Worker {
  private readonly topicHandlers = new Map<string, ContextualHandler>();
  private readonly topicDrivers = new Map<string, string>();
  private readonly typeHandlers = new Map<string, ContextualHandler>();
  private readonly ruleHandlers = new Map<string, ContextualHandler>();
  private readonly allowedTopics = new Set<string>();
  private readonly driverSubs = new Map<string, Subscriber>();
  private readonly topics: string[] = [];

  private subscriber?: Subscriber;
  private codec: Codec;
  private retry: RetryPolicy;
  private retryCount: number;
  private logger: Logger;
  private middleware: Middleware[];
  private listeners: Listener[];
  private clientProvider?: ClientProvider;
  private semaphore: Semaphore;
  private endpoint: string;
  private apiKey: string;
  private oauth2Config?: OAuth2Config;
  private tenantId: string;
  private defaultDriverId: string;
  private validate: boolean;

  constructor(opts: WorkerOptions = {}) {
    this.subscriber = opts.subscriber;
    this.codec = opts.codec ?? new DefaultCodec();
    this.retry = opts.retry ?? new NoRetry();
    this.retryCount = normalizeRetryCount(opts.retryCount);
    this.logger = opts.logger ?? defaultLogger;
    this.middleware = opts.middleware ?? [];
    this.listeners = opts.listeners ?? [];
    this.clientProvider = opts.clientProvider;
    this.endpoint = resolveEndpoint(opts.endpoint);
    this.apiKey = resolveApiKey(opts.apiKey);
    this.oauth2Config = resolveOAuth2Config(opts.oauth2Config);
    this.tenantId = resolveTenantId(opts.tenantId);
    this.defaultDriverId = (opts.defaultDriverId ?? "").trim();
    this.validate = opts.validateTopics !== false;
    this.semaphore = new Semaphore(normalizeConcurrency(opts.concurrency));

    if (opts.topics) {
      this.addTopics(opts.topics);
    }
    this.bindClientProvider();
  }

  static new(...options: WorkerOption[]): Worker {
    const wk = new Worker();
    for (const opt of options) {
      opt(wk);
    }
    return wk;
  }

  HandleTopic(topic: string, handler: Handler): void;
  HandleTopic(topic: string, driverId: string, handler: Handler): void;
  HandleTopic(topic: string, driverOrHandler: string | Handler, handler?: Handler): void {
    if (typeof driverOrHandler === "function") {
      this.handleTopic(topic, driverOrHandler);
    } else {
      this.handleTopic(topic, driverOrHandler, handler as Handler);
    }
  }

  handleTopic(topic: string, handler: Handler): void;
  handleTopic(topic: string, driverId: string, handler: Handler): void;
  handleTopic(topic: string, driverOrHandler: string | Handler, handler?: Handler): void {
    const trimmed = (topic ?? "").trim();
    if (!trimmed) {
      return;
    }
    if (this.allowedTopics.size > 0 && !this.allowedTopics.has(trimmed)) {
      logPrintf(this.logger, "handler topic not subscribed: %s", trimmed);
      return;
    }

    let driverId = "";
    let resolvedHandler: Handler | undefined;
    if (typeof driverOrHandler === "function") {
      resolvedHandler = driverOrHandler;
    } else {
      driverId = (driverOrHandler ?? "").trim();
      resolvedHandler = handler;
    }
    if (!resolvedHandler) {
      return;
    }
    if (!driverId) {
      driverId = this.defaultDriverId;
    }
    if (!driverId && !this.subscriber) {
      logPrintf(this.logger, "driver id required for topic: %s", trimmed);
      return;
    }
    this.topicHandlers.set(trimmed, toContextHandler(resolvedHandler));
    if (driverId) {
      this.topicDrivers.set(trimmed, driverId);
    }
    this.topics.push(trimmed);
  }

  handleType(eventType: string, handler: Handler): void {
    const trimmed = (eventType ?? "").trim();
    if (!trimmed || !handler) {
      return;
    }
    this.typeHandlers.set(trimmed, toContextHandler(handler));
  }

  HandleType(eventType: string, handler: Handler): void {
    this.handleType(eventType, handler);
  }

  handleRule(ruleId: string, handler: Handler): void {
    const trimmed = (ruleId ?? "").trim();
    if (!trimmed || !handler) {
      return;
    }
    this.ruleHandlers.set(trimmed, toContextHandler(handler));
  }

  HandleRule(ruleId: string, handler: Handler): void {
    this.handleRule(ruleId, handler);
  }

  async run(ctx?: WorkerContext | AbortSignal): Promise<void> {
    const baseCtx = this.resolveContext(ctx);
    await this.prepareRuleSubscriptions(baseCtx);
    if (this.topics.length === 0) {
      throw new Error("at least one topic is required");
    }
    const abortPromise = makeAbortPromise(baseCtx.signal, () => this.close());

    if (this.subscriber) {
      if (this.validate) {
        await this.validateTopics(baseCtx);
      }
      await Promise.race([this.runWithSubscriber(baseCtx, this.subscriber, unique(this.topics)), abortPromise]);
      return;
    }

    const driverTopics = this.topicsByDriver();
    if (this.validate) {
      await this.validateTopics(baseCtx);
    }
    await this.buildDriverSubscribers(baseCtx, driverTopics);
    await Promise.race([this.runDriverSubscribers(baseCtx, driverTopics), abortPromise]);
  }

  Run(ctx?: WorkerContext | AbortSignal): Promise<void> {
    return this.run(ctx);
  }

  async close(): Promise<void> {
    if (this.subscriber) {
      await this.subscriber.close();
      return;
    }
    for (const sub of this.driverSubs.values()) {
      await sub.close();
    }
  }

  Close(): Promise<void> {
    return this.close();
  }

  apply(options: WorkerOptions): void {
    if (options.subscriber) {
      this.subscriber = options.subscriber;
    }
    if (options.codec) {
      this.codec = options.codec;
    }
    if (options.retry) {
      this.retry = options.retry;
    }
    if (options.retryCount !== undefined) {
      this.retryCount = normalizeRetryCount(options.retryCount);
    }
    if (options.logger) {
      this.logger = options.logger;
    }
    if (options.middleware) {
      this.middleware.push(...options.middleware);
    }
    if (options.listeners) {
      this.listeners.push(...options.listeners);
    }
    if (options.clientProvider) {
      this.clientProvider = options.clientProvider;
    }
    if (options.endpoint) {
      this.endpoint = resolveEndpoint(options.endpoint);
    }
    if (options.apiKey) {
      this.apiKey = resolveApiKey(options.apiKey);
    }
    if (options.oauth2Config) {
      this.oauth2Config = resolveOAuth2Config(options.oauth2Config);
    }
    if (options.tenantId) {
      this.tenantId = resolveTenantId(options.tenantId);
    }
    if (options.defaultDriverId) {
      this.defaultDriverId = options.defaultDriverId.trim();
    }
    if (options.validateTopics !== undefined) {
      this.validate = options.validateTopics;
    }
    if (options.concurrency) {
      this.semaphore = new Semaphore(normalizeConcurrency(options.concurrency));
    }
    if (options.topics) {
      this.addTopics(options.topics);
    }
    if (options.clientProvider || options.oauth2Config || options.endpoint || options.apiKey) {
      this.bindClientProvider();
    }
  }

  Apply(options: WorkerOptions): void {
    this.apply(options);
  }

  private addTopics(topics: string[]): void {
    for (const topic of topics) {
      const trimmed = (topic ?? "").trim();
      if (!trimmed) {
        continue;
      }
      this.topics.push(trimmed);
      this.allowedTopics.add(trimmed);
    }
  }

  private async runWithSubscriber(ctx: WorkerContext, sub: Subscriber, topics: string[]): Promise<void> {
    this.notifyStart(ctx);
    try {
      await Promise.all(topics.map((topic) => this.runTopicSubscriber(ctx, sub, topic)));
    } finally {
      this.notifyExit(ctx);
    }
  }

  private async runDriverSubscribers(ctx: WorkerContext, driverTopics: Map<string, string[]>): Promise<void> {
    this.notifyStart(ctx);
    try {
      const tasks: Array<Promise<void>> = [];
      for (const [driverId, topics] of driverTopics.entries()) {
        const sub = this.driverSubs.get(driverId);
        if (!sub) {
          throw new Error(`subscriber not initialized for driver: ${driverId}`);
        }
        for (const topic of unique(topics)) {
          tasks.push(this.runTopicSubscriber(ctx, sub, topic));
        }
      }
      await Promise.all(tasks);
    } finally {
      this.notifyExit(ctx);
    }
  }

  private async runTopicSubscriber(ctx: WorkerContext, sub: Subscriber, topic: string): Promise<void> {
    await sub.start(topic, async (msg: RelaybusMessage) => {
      await this.semaphore.use(async () => {
        const shouldNack = await this.handleMessage(ctx, topic, msg);
        if (shouldNack && shouldRequeue(msg)) {
          throw new Error("message nack requested");
        }
      });
    });
  }

  private topicsByDriver(): Map<string, string[]> {
    const topics = unique(this.topics);
    if (this.topicDrivers.size === 0) {
      if (this.defaultDriverId) {
        return new Map([[this.defaultDriverId, topics]]);
      }
      throw new Error("driver id is required for topics");
    }
    const out = new Map<string, string[]>();
    for (const topic of topics) {
      let driverId = this.topicDrivers.get(topic);
      if (!driverId) {
        driverId = this.defaultDriverId;
      }
      const trimmed = (driverId ?? "").trim();
      if (!trimmed) {
        throw new Error(`driver id is required for topic: ${topic}`);
      }
      const list = out.get(trimmed) ?? [];
      list.push(topic);
      out.set(trimmed, list);
    }
    return out;
  }

  private async buildDriverSubscribers(ctx: WorkerContext, driverTopics: Map<string, string[]>): Promise<void> {
    for (const driverId of driverTopics.keys()) {
      if (this.driverSubs.has(driverId)) {
        continue;
      }
      const record = await this.driversClient().getDriverById(driverId, ctx);
      if (!record) {
        throw new Error(`driver not found: ${driverId}`);
      }
      if (!record.enabled) {
        throw new Error(`driver is disabled: ${driverId}`);
      }
      const cfg = subscriberConfigFromDriver(record.name, record.configJson);
      const sub = buildSubscriber(cfg);
      this.driverSubs.set(driverId, sub);
    }
  }

  private async validateTopics(ctx: WorkerContext): Promise<void> {
    const rules = await this.rulesClient().listRules(ctx);
    if (rules.length === 0) {
      throw new Error("no rules available from api");
    }

    const allowedTopics = new Set<string>();
    const allowedByDriver = new Map<string, Set<string>>();
    for (const rule of rules) {
      for (const topic of rule.emit) {
        const trimmed = (topic ?? "").trim();
        if (!trimmed) {
          continue;
        }
        allowedTopics.add(trimmed);
        const driverId = (rule.driverId ?? "").trim();
        if (!driverId) {
          continue;
        }
        if (!allowedByDriver.has(driverId)) {
          allowedByDriver.set(driverId, new Set<string>());
        }
        allowedByDriver.get(driverId)?.add(trimmed);
      }
    }
    if (allowedTopics.size === 0) {
      throw new Error("no topics available from rules");
    }

    const topics = unique(this.topics);
    if (this.subscriber) {
      for (const topic of topics) {
        if (!allowedTopics.has(topic)) {
          throw new Error(`unknown topic: ${topic}`);
        }
      }
      return;
    }

    for (const topic of topics) {
      let driverId = this.topicDrivers.get(topic);
      if (!driverId) {
        driverId = this.defaultDriverId;
      }
      if (!driverId) {
        throw new Error(`driver id is required for topic: ${topic}`);
      }
      const allowed = allowedByDriver.get(driverId);
      if (!allowed) {
        throw new Error(`driver not configured on any rule: ${driverId}`);
      }
      if (!allowed.has(topic)) {
        throw new Error(`topic ${topic} not configured for driver ${driverId}`);
      }
    }
  }

  private async prepareRuleSubscriptions(ctx: WorkerContext): Promise<void> {
    if (this.ruleHandlers.size === 0) {
      return;
    }
    const client = this.rulesClient();
    for (const [ruleId, handler] of this.ruleHandlers.entries()) {
      const record = await client.getRule(ruleId, ctx);
      if (record.emit.length === 0) {
        throw new Error(`rule ${ruleId} has no emit topic`);
      }
      const topic = (record.emit[0] ?? "").trim();
      if (!topic) {
        throw new Error(`rule ${ruleId} emit topic empty`);
      }
      const driverId = (record.driverId ?? "").trim();
      if (!driverId) {
        throw new Error(`rule ${ruleId} driver_id is required`);
      }
      if (this.topicHandlers.has(topic)) {
        logPrintf(this.logger, "overwriting handler for topic=%s due to rule=%s", topic, ruleId);
      }
      this.topicHandlers.set(topic, handler);
      this.topicDrivers.set(topic, driverId);
      this.topics.push(topic);
    }
  }

  private async handleMessage(ctx: WorkerContext, topic: string, msg: RelaybusMessage): Promise<boolean> {
    const meta = (msg.metadata ?? (msg as Record<string, unknown>)["meta"]) as
      | Record<string, string>
      | undefined;
    const logId = meta?.[MetadataKeyLogID] ?? "";
    let event: Event | undefined;

    try {
      event = this.codec.decode(topic, msg);
    } catch (err) {
      const error = normalizeError(err);
      logPrintf(this.logger, "decode failed: %s", error.message);
      await this.updateEventLogStatus(ctx, logId, EventLogStatusFailed, error);
      this.notifyError(ctx, undefined, error);
      const decision = normalizeRetryDecision(callRetryPolicy(this.retry, ctx, undefined, error));
      return decision.retry || decision.nack;
    }

    const eventCtx = this.buildContext(ctx, topic, msg);
    if (this.clientProvider) {
      try {
        event.client = await resolveClientProvider(this.clientProvider)(eventCtx, event);
      } catch (err) {
        const error = normalizeError(err);
        logPrintf(this.logger, "client init failed: %s", error.message);
        await this.updateEventLogStatus(eventCtx, logId, EventLogStatusFailed, error);
        this.notifyError(eventCtx, event, error);
        const decision = normalizeRetryDecision(callRetryPolicy(this.retry, eventCtx, event, error));
        return decision.retry || decision.nack;
      }
    }

    const reqId = event.metadata[MetadataKeyRequestID];
    if (reqId) {
      logPrintf(this.logger, "request_id=%s topic=%s provider=%s type=%s", reqId, event.topic, event.provider, event.type);
    }

    this.notifyMessageStart(eventCtx, event);

    const handler = this.topicHandlers.get(topic) ?? this.typeHandlers.get(event.type);
    if (!handler) {
      logPrintf(this.logger, "no handler for topic=%s type=%s", topic, event.type);
      this.notifyMessageFinish(eventCtx, event, undefined);
      await this.updateEventLogStatus(eventCtx, logId, EventLogStatusSuccess);
      return false;
    }

    const wrapped = this.wrap(handler);
    let lastError: Error | undefined;
    const attempts = this.retryCount + 1;
    for (let i = 0; i < attempts; i += 1) {
      try {
        await wrapped(eventCtx, event);
        lastError = undefined;
        break;
      } catch (err) {
        lastError = normalizeError(err);
      }
    }
    if (!lastError) {
      this.notifyMessageFinish(eventCtx, event, undefined);
      await this.updateEventLogStatus(eventCtx, logId, EventLogStatusSuccess);
      return false;
    }
    this.notifyMessageFinish(eventCtx, event, lastError);
    this.notifyError(eventCtx, event, lastError);
    await this.updateEventLogStatus(eventCtx, logId, EventLogStatusFailed, lastError);
    const decision = normalizeRetryDecision(callRetryPolicy(this.retry, eventCtx, event, lastError));
    return decision.retry || decision.nack;
  }

  private wrap(handler: ContextualHandler): ContextualHandler {
    let wrapped = handler;
    for (let i = this.middleware.length - 1; i >= 0; i -= 1) {
      wrapped = this.middleware[i](wrapped);
    }
    return wrapped;
  }

  private buildContext(base: WorkerContext, topic: string, msg: RelaybusMessage): WorkerContext {
    const meta = (msg.metadata ?? (msg as Record<string, unknown>)["meta"]) as
      | Record<string, string>
      | undefined;
    const metadataTenant = (meta?.[MetadataKeyTenantID] ?? "").trim();
    const baseTenant = (base.tenantId ?? "").trim();
    return {
      tenantId: metadataTenant || baseTenant,
      signal: base.signal,
      topic,
      requestId: meta?.[MetadataKeyRequestID],
      logId: meta?.[MetadataKeyLogID],
    };
  }

  private resolveContext(ctx?: WorkerContext | AbortSignal): WorkerContext {
    if (!ctx) {
      return { tenantId: this.tenantId };
    }
    if (isAbortSignal(ctx)) {
      return { tenantId: this.tenantId, signal: ctx };
    }
    return { tenantId: ctx.tenantId ?? this.tenantId, signal: ctx.signal };
  }

  private notifyStart(ctx: WorkerContext): void {
    for (const listener of this.listeners) {
      listener.onStart?.(ctx);
      listener.OnStart?.(ctx);
    }
  }

  private notifyExit(ctx: WorkerContext): void {
    for (const listener of this.listeners) {
      listener.onExit?.(ctx);
      listener.OnExit?.(ctx);
    }
  }

  private notifyMessageStart(ctx: WorkerContext, evt: Event): void {
    for (const listener of this.listeners) {
      listener.onMessageStart?.(ctx, evt);
      listener.OnMessageStart?.(ctx, evt);
    }
  }

  private notifyMessageFinish(ctx: WorkerContext, evt: Event, err?: Error): void {
    for (const listener of this.listeners) {
      listener.onMessageFinish?.(ctx, evt, err);
      listener.OnMessageFinish?.(ctx, evt, err);
    }
  }

  private notifyError(ctx: WorkerContext, evt: Event | undefined, err: Error): void {
    for (const listener of this.listeners) {
      listener.onError?.(ctx, evt, err);
      listener.OnError?.(ctx, evt, err);
    }
  }

  private driversClient(): DriversClient {
    return new DriversClient(this.apiClientOptions());
  }

  private rulesClient(): RulesClient {
    return new RulesClient(this.apiClientOptions());
  }

  private eventLogsClient(): EventLogsClient {
    return new EventLogsClient(this.apiClientOptions());
  }

  private apiClientOptions(): APIClientOptions {
    return {
      baseUrl: this.endpoint,
      apiKey: this.apiKey,
      oauth2Config: this.oauth2Config,
      tenantId: this.tenantId,
    };
  }

  private async updateEventLogStatus(
    ctx: WorkerContext,
    logId: string,
    status: string,
    err?: Error,
  ): Promise<void> {
    if (!logId) {
      logPrintf(this.logger, "event log update skipped: empty log_id");
      return;
    }
    try {
      logPrintf(
        this.logger,
        "event log update request log_id=%s status=%s err=%s",
        logId,
        status,
        err?.message ?? "",
      );
      if (ctx.tenantId) {
        logPrintf(this.logger, "event log update tenant=%s", ctx.tenantId);
      }
      await this.eventLogsClient().updateStatus(logId, status, err?.message, ctx);
      logPrintf(
        this.logger,
        "event log update ok log_id=%s status=%s",
        logId,
        status,
      );
    } catch (updateErr) {
      const error = normalizeError(updateErr);
      logPrintf(this.logger, "event log update failed: %s", error.message);
    }
  }

  private bindClientProvider(): void {
    if (!this.clientProvider) {
      return;
    }
    const provider = this.clientProvider as unknown as {
      bindAPIClient?: (opts: APIClientOptions) => void;
      BindAPIClient?: (opts: APIClientOptions) => void;
    };
    const opts = this.apiClientOptions();
    if (provider.bindAPIClient) {
      provider.bindAPIClient(opts);
    } else if (provider.BindAPIClient) {
      provider.BindAPIClient(opts);
    }
  }
}

export function New(...options: WorkerOption[]): Worker {
  return Worker.new(...options);
}

export function WithSubscriber(subscriber: Subscriber): WorkerOption {
  return (wk) => wk.apply({ subscriber });
}

export function WithTopics(...topics: string[]): WorkerOption {
  return (wk) => wk.apply({ topics });
}

export function WithConcurrency(concurrency: number): WorkerOption {
  return (wk) => wk.apply({ concurrency });
}

export function WithCodec(codec: Codec): WorkerOption {
  return (wk) => wk.apply({ codec });
}

export function WithMiddleware(...middleware: Middleware[]): WorkerOption {
  return (wk) => wk.apply({ middleware });
}

export function WithRetry(retry: RetryPolicy): WorkerOption {
  return (wk) => wk.apply({ retry });
}

export function WithRetryCount(retryCount: number): WorkerOption {
  return (wk) => wk.apply({ retryCount });
}

export function WithLogger(logger: Logger): WorkerOption {
  return (wk) => wk.apply({ logger });
}

export function WithClientProvider(clientProvider: ClientProvider): WorkerOption {
  return (wk) => wk.apply({ clientProvider });
}

export function WithListener(listener: Listener): WorkerOption {
  return (wk) => wk.apply({ listeners: [listener] });
}

export function WithEndpoint(endpoint: string): WorkerOption {
  return (wk) => wk.apply({ endpoint });
}

export function WithAPIKey(apiKey: string): WorkerOption {
  return (wk) => wk.apply({ apiKey });
}

export function WithOAuth2Config(oauth2Config: OAuth2Config): WorkerOption {
  return (wk) => wk.apply({ oauth2Config });
}

export function WithTenant(tenantId: string): WorkerOption {
  return (wk) => wk.apply({ tenantId });
}

export function WithDefaultDriver(driverId: string): WorkerOption {
  return (wk) => wk.apply({ defaultDriverId: driverId });
}

export function WithValidateTopics(validate: boolean): WorkerOption {
  return (wk) => wk.apply({ validateTopics: validate });
}

const defaultLogger: Logger = {
  printf(format: string, ...args: unknown[]) {
    if (args.length === 0) {
      console.log(`relaymesh/worker ${format}`);
      return;
    }
    console.log(`relaymesh/worker ${format}`, ...args);
  },
};

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

function resolveApiKey(explicit?: string): string {
  const trimmed = (explicit ?? "").trim();
  if (trimmed) {
    return trimmed;
  }
  return envValue("RELAYMESH_API_KEY");
}

function resolveTenantId(explicit?: string): string {
  const trimmed = (explicit ?? "").trim();
  if (trimmed) {
    return trimmed;
  }
  return envValue("RELAYMESH_TENANT_ID");
}

function envValue(key: string): string {
  return (process.env[key] ?? "").trim();
}

function normalizeConcurrency(value?: number): number {
  if (!value || value < 1) {
    return 10;
  }
  return Math.floor(value);
}

function normalizeRetryCount(value?: number): number {
  if (value === undefined || value === null) {
    return 0;
  }
  const num = Math.floor(Number(value));
  if (Number.isNaN(num) || num < 0) {
    return 0;
  }
  return num;
}

function unique(values: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const value of values) {
    if (seen.has(value)) {
      continue;
    }
    seen.add(value);
    out.push(value);
  }
  return out;
}

function normalizeError(err: unknown): Error {
  if (err instanceof Error) {
    return err;
  }
  return new Error(String(err));
}

function callRetryPolicy(
  policy: RetryPolicy,
  ctx: WorkerContext,
  evt: Event | undefined,
  err: Error,
) {
  if (policy.OnError) {
    return policy.OnError(ctx, evt, err);
  }
  if (policy.onError) {
    return policy.onError(ctx, evt, err);
  }
  return { retry: false, nack: true };
}

function logPrintf(logger: Logger, format: string, ...args: unknown[]): void {
  if (logger.Printf) {
    logger.Printf(format, ...args);
    return;
  }
  if (logger.printf) {
    logger.printf(format, ...args);
    return;
  }
  if (args.length === 0) {
    console.log(`githook/worker ${format}`);
    return;
  }
  console.log(`githook/worker ${format}`, ...args);
}

function resolveClientProvider(provider: ClientProvider): (ctx: WorkerContext, evt: Event) => Promise<unknown> {
  if (provider.client) {
    return async (ctx, evt) => provider.client?.(ctx, evt);
  }
  if (provider.Client) {
    return async (ctx, evt) => provider.Client?.(ctx, evt);
  }
  return async () => undefined;
}

function toContextHandler(handler: Handler): ContextualHandler {
  if (handler.length >= 2) {
    return handler as (ctx: WorkerContext, event: Event) => Promise<void> | void;
  }
  return (_ctx: WorkerContext, event: Event) =>
    (handler as (event: Event) => Promise<void> | void)(event);
}

function shouldRequeue(msg: RelaybusMessage): boolean {
  const driver = (msg.metadata?.[MetadataKeyDriver] ?? "").toLowerCase();
  return driver === "amqp";
}

function isAbortSignal(value: unknown): value is AbortSignal {
  return !!value && typeof (value as AbortSignal).aborted === "boolean";
}

function makeAbortPromise(signal: AbortSignal | undefined, onAbort: () => void): Promise<void> {
  if (!signal) {
    return new Promise(() => {});
  }
  if (signal.aborted) {
    onAbort();
    return Promise.reject(new Error("aborted"));
  }
  return new Promise((_, reject) => {
    signal.addEventListener(
      "abort",
      () => {
        onAbort();
        reject(new Error("aborted"));
      },
      { once: true },
    );
  });
}

class Semaphore {
  private available: number;
  private queue: Array<() => void> = [];

  constructor(private readonly size: number) {
    this.available = size;
  }

  async use(task: () => Promise<void>): Promise<void> {
    const release = await this.acquire();
    try {
      await task();
    } finally {
      release();
    }
  }

  private acquire(): Promise<() => void> {
    return new Promise((resolve) => {
      const attempt = () => {
        if (this.available > 0) {
          this.available -= 1;
          resolve(() => {
            this.available += 1;
            const next = this.queue.shift();
            if (next) {
              next();
            }
          });
          return;
        }
        this.queue.push(attempt);
      };
      attempt();
    });
  }
}
