import inspect
import threading
from dataclasses import dataclass
from typing import Any, Callable, Dict, List, Optional, Protocol, Sequence, Set, Union

from .api import (
    APIClientOptions,
    DriversClient,
    EventLogsClient,
    RulesClient,
    resolve_api_key,
    resolve_endpoint,
    resolve_tenant_id,
)
from .client import ClientProvider
from .codec import Codec, DefaultCodec
from .context import WorkerContext
from .event import Event
from .event_log_status import EVENT_LOG_STATUS_FAILED, EVENT_LOG_STATUS_SUCCESS
from .listener import Listener
from .metadata import (
    METADATA_KEY_DRIVER,
    METADATA_KEY_LOG_ID,
    METADATA_KEY_REQUEST_ID,
    METADATA_KEY_TENANT_ID,
)
from .oauth2 import OAuth2Config, resolve_oauth2_config
from .retry import NoRetry, RetryPolicy, normalize_retry_decision
from .subscriber import Subscriber, build_subscriber, subscriber_config_from_driver
from .types import RelaybusMessage, coerce_message

Handler = Union[Callable[[WorkerContext, Event], Any], Callable[[Event], Any]]
ContextualHandler = Callable[[WorkerContext, Event], Any]
Middleware = Callable[[ContextualHandler], ContextualHandler]


def _normalize_retry_count(value: Optional[int]) -> int:
    if value is None:
        return 0
    try:
        num = int(value)
    except Exception:
        return 0
    if num < 0:
        return 0
    return num


class Logger(Protocol):
    def printf(self, fmt: str, *args: Any) -> None: ...

    def Printf(self, fmt: str, *args: Any) -> None: ...


@dataclass
class WorkerOptions:
    subscriber: Optional[Subscriber] = None
    topics: Optional[List[str]] = None
    codec: Optional[Codec] = None
    logger: Optional[Logger] = None
    concurrency: Optional[int] = None
    middleware: Optional[List[Middleware]] = None
    retry: Optional[RetryPolicy] = None
    retry_count: Optional[int] = None
    listeners: Optional[List[Listener]] = None
    client_provider: Optional[ClientProvider] = None
    endpoint: Optional[str] = None
    api_key: Optional[str] = None
    oauth2_config: Optional[OAuth2Config] = None
    tenant_id: Optional[str] = None
    default_driver_id: Optional[str] = None
    validate_topics: Optional[bool] = None


WorkerOption = Callable[["Worker"], None]


class Worker:
    def __init__(self, opts: Optional[WorkerOptions] = None) -> None:
        options = opts or WorkerOptions()
        self.subscriber = options.subscriber
        self.codec = options.codec or DefaultCodec()
        self.retry = options.retry or NoRetry()
        self.retry_count = _normalize_retry_count(options.retry_count)
        self.logger = options.logger or _StdLogger()
        self.concurrency = max(1, int(options.concurrency or 10))
        self.middleware = list(options.middleware or [])
        self.listeners = list(options.listeners or [])
        self.client_provider = options.client_provider
        self.endpoint = resolve_endpoint(options.endpoint or "")
        self.api_key = resolve_api_key(options.api_key or "")
        self.oauth2_config = resolve_oauth2_config(options.oauth2_config)
        self.tenant_id = resolve_tenant_id(options.tenant_id or "")
        self.default_driver_id = (options.default_driver_id or "").strip()
        self.validate = (
            True if options.validate_topics is None else bool(options.validate_topics)
        )

        self.topic_handlers: Dict[str, ContextualHandler] = {}
        self.topic_drivers: Dict[str, str] = {}
        self.type_handlers: Dict[str, ContextualHandler] = {}
        self.rule_handlers: Dict[str, ContextualHandler] = {}
        self.allowed_topics: Set[str] = set()
        self.driver_subs: Dict[str, Subscriber] = {}
        self.topics: List[str] = []
        self.semaphore = threading.Semaphore(self.concurrency)

        if options.topics:
            self.add_topics(options.topics)
        self.bind_client_provider()

    @classmethod
    def new(cls, *options: WorkerOption) -> "Worker":
        wk = cls()
        for opt in options:
            opt(wk)
        return wk

    def apply(self, options: WorkerOptions) -> None:
        if options.subscriber is not None:
            self.subscriber = options.subscriber
        if options.codec is not None:
            self.codec = options.codec
        if options.logger is not None:
            self.logger = options.logger
        if options.concurrency is not None:
            self.concurrency = max(1, int(options.concurrency))
            self.semaphore = threading.Semaphore(self.concurrency)
        if options.middleware is not None:
            self.middleware = list(options.middleware)
        if options.retry is not None:
            self.retry = options.retry
        if options.retry_count is not None:
            self.retry_count = _normalize_retry_count(options.retry_count)
        if options.listeners is not None:
            self.listeners = list(options.listeners)
        if options.client_provider is not None:
            self.client_provider = options.client_provider
        if options.endpoint is not None:
            self.endpoint = resolve_endpoint(options.endpoint)
        if options.api_key is not None:
            self.api_key = resolve_api_key(options.api_key)
        if options.oauth2_config is not None:
            self.oauth2_config = resolve_oauth2_config(options.oauth2_config)
        if options.tenant_id is not None:
            self.tenant_id = resolve_tenant_id(options.tenant_id)
        if options.default_driver_id is not None:
            self.default_driver_id = (options.default_driver_id or "").strip()
        if options.validate_topics is not None:
            self.validate = bool(options.validate_topics)
        if options.topics:
            self.add_topics(options.topics)
        if (
            options.client_provider is not None
            or options.oauth2_config is not None
            or options.endpoint is not None
            or options.api_key is not None
        ):
            self.bind_client_provider()

    def Apply(self, options: WorkerOptions) -> None:
        self.apply(options)

    def add_topics(self, topics: Sequence[str]) -> None:
        for topic in topics:
            trimmed = (topic or "").strip()
            if not trimmed:
                continue
            self.topics.append(trimmed)
            self.allowed_topics.add(trimmed)

    def handle_topic(
        self,
        topic: str,
        driver_or_handler: Union[str, Handler],
        handler: Optional[Handler] = None,
    ) -> None:
        trimmed = (topic or "").strip()
        if not trimmed:
            return
        if self.allowed_topics and trimmed not in self.allowed_topics:
            _log_printf(self.logger, "handler topic not subscribed: %s", trimmed)
            return

        driver_id = ""
        resolved_handler: Optional[Handler]
        if callable(driver_or_handler) and handler is None:
            resolved_handler = driver_or_handler  # type: ignore[assignment]
        else:
            driver_id = (driver_or_handler or "").strip()  # type: ignore[arg-type]
            resolved_handler = handler
        if resolved_handler is None:
            return
        if not driver_id:
            driver_id = self.default_driver_id
        if not driver_id and self.subscriber is None:
            _log_printf(self.logger, "driver id required for topic: %s", trimmed)
            return

        self.topic_handlers[trimmed] = _to_context_handler(resolved_handler)
        if driver_id:
            self.topic_drivers[trimmed] = driver_id
        self.topics.append(trimmed)

    def HandleTopic(
        self,
        topic: str,
        driver_or_handler: Union[str, Handler],
        handler: Optional[Handler] = None,
    ) -> None:
        self.handle_topic(topic, driver_or_handler, handler)

    def handle_type(self, event_type: str, handler: Handler) -> None:
        trimmed = (event_type or "").strip()
        if not trimmed or handler is None:
            return
        self.type_handlers[trimmed] = _to_context_handler(handler)

    def HandleType(self, event_type: str, handler: Handler) -> None:
        self.handle_type(event_type, handler)

    def handle_rule(self, rule_id: str, handler: Handler) -> None:
        trimmed = (rule_id or "").strip()
        if not trimmed or handler is None:
            return
        self.rule_handlers[trimmed] = _to_context_handler(handler)

    def HandleRule(self, rule_id: str, handler: Handler) -> None:
        self.handle_rule(rule_id, handler)

    def run(self, ctx: Optional[Union[WorkerContext, threading.Event]] = None) -> None:
        base_ctx = self.resolve_context(ctx)
        self.prepare_rule_subscriptions(base_ctx)
        if not self.topics:
            raise ValueError("at least one topic is required")

        if self.subscriber is not None:
            if self.validate:
                self.validate_topics(base_ctx)
            self.run_with_subscriber(base_ctx, self.subscriber, _unique(self.topics))
            return

        driver_topics = self.topics_by_driver()
        if self.validate:
            self.validate_topics(base_ctx)
        self.build_driver_subscribers(base_ctx, driver_topics)
        self.run_driver_subscribers(base_ctx, driver_topics)

    def Run(self, ctx: Optional[Union[WorkerContext, threading.Event]] = None) -> None:
        self.run(ctx)

    def close(self) -> None:
        if self.subscriber is None:
            for sub in list(self.driver_subs.values()):
                if sub is None:
                    continue
                try:
                    sub.close()
                except Exception:
                    continue
            return
        try:
            self.subscriber.close()
        except Exception:
            return None

    def Close(self) -> None:
        self.close()

    def resolve_context(
        self, ctx: Optional[Union[WorkerContext, threading.Event]]
    ) -> WorkerContext:
        if ctx is None:
            return WorkerContext(tenant_id=self.tenant_id)
        if isinstance(ctx, WorkerContext):
            tenant_id = ctx.tenant_id or self.tenant_id
            return WorkerContext(
                tenant_id=tenant_id,
                signal=ctx.signal,
                topic=ctx.topic,
                request_id=ctx.request_id,
                log_id=ctx.log_id,
            )
        if isinstance(ctx, threading.Event):
            return WorkerContext(tenant_id=self.tenant_id, signal=ctx)
        return WorkerContext(tenant_id=self.tenant_id)

    def run_with_subscriber(
        self, ctx: WorkerContext, sub: Subscriber, topics: List[str]
    ) -> None:
        self.notify_start(ctx)
        try:
            self._run_topic_subscribers(ctx, sub, topics)
        finally:
            self.notify_exit(ctx)

    def run_driver_subscribers(
        self, ctx: WorkerContext, driver_topics: Dict[str, List[str]]
    ) -> None:
        self.notify_start(ctx)
        try:
            tasks: List[Callable[[], None]] = []
            for driver_id, topics in driver_topics.items():
                sub = self.driver_subs.get(driver_id)
                if sub is None:
                    raise ValueError(
                        f"subscriber not initialized for driver: {driver_id}"
                    )
                for topic in _unique(topics):
                    tasks.append(
                        lambda sub=sub, topic=topic: self._run_topic_subscriber(
                            ctx, sub, topic
                        )
                    )
            self._run_tasks(ctx, tasks)
        finally:
            self.notify_exit(ctx)

    def _run_topic_subscribers(
        self, ctx: WorkerContext, sub: Subscriber, topics: List[str]
    ) -> None:
        tasks = [
            lambda sub=sub, topic=topic: self._run_topic_subscriber(ctx, sub, topic)
            for topic in topics
        ]
        self._run_tasks(ctx, tasks)

    def _run_tasks(self, ctx: WorkerContext, tasks: List[Callable[[], None]]) -> None:
        errors: List[Exception] = []
        lock = threading.Lock()

        def wrap(task: Callable[[], None]) -> None:
            try:
                task()
            except Exception as exc:
                with lock:
                    errors.append(exc)
                if ctx.signal is not None:
                    ctx.signal.set()

        threads = [
            threading.Thread(target=wrap, args=(task,), daemon=True) for task in tasks
        ]
        for t in threads:
            t.start()

        if ctx.signal is not None:
            watcher = threading.Thread(
                target=self._wait_for_signal, args=(ctx.signal,), daemon=True
            )
            watcher.start()

        for t in threads:
            t.join()

        if errors:
            raise errors[0]

    def _wait_for_signal(self, signal: threading.Event) -> None:
        signal.wait()
        self.close()

    def _run_topic_subscriber(
        self, ctx: WorkerContext, sub: Subscriber, topic: str
    ) -> None:
        def handler(msg: RelaybusMessage) -> Optional[bool]:
            with self.semaphore:
                relay_msg = coerce_message(msg)
                should_nack = self.handle_message(ctx, topic, relay_msg)
                if should_nack and _should_requeue(relay_msg):
                    return True
                return False

        sub.start(topic, handler)

    def topics_by_driver(self) -> Dict[str, List[str]]:
        if not self.topic_drivers:
            default_driver = (self.default_driver_id or "").strip()
            if default_driver:
                return {default_driver: _unique(self.topics)}
            raise ValueError("driver id is required for topics")
        out: Dict[str, List[str]] = {}
        for topic, driver_id in self.topic_drivers.items():
            trimmed = (driver_id or "").strip()
            if not trimmed:
                raise ValueError(f"driver id is required for topic: {topic}")
            out.setdefault(trimmed, []).append(topic)
        return out

    def build_driver_subscribers(
        self, ctx: WorkerContext, driver_topics: Dict[str, List[str]]
    ) -> None:
        for driver_id in driver_topics:
            if driver_id in self.driver_subs:
                continue
            record = self.drivers_client().get_driver_by_id(driver_id, ctx)
            if record is None:
                raise ValueError(f"driver not found: {driver_id}")
            if not record.enabled:
                raise ValueError(f"driver is disabled: {driver_id}")
            cfg = subscriber_config_from_driver(record.name, record.config_json)
            sub = build_subscriber(cfg)
            self.driver_subs[driver_id] = sub

    def handle_message(
        self, ctx: WorkerContext, topic: str, msg: RelaybusMessage
    ) -> bool:
        metadata = msg.metadata or getattr(msg, "meta", None) or {}
        log_id = metadata.get(METADATA_KEY_LOG_ID, "")
        try:
            evt = self.codec.decode(topic, msg)
        except Exception as err:
            _log_printf(self.logger, "decode failed: %s", err)
            self.update_event_log_status(ctx, log_id, EVENT_LOG_STATUS_FAILED, err)
            self.notify_error(ctx, None, err)
            decision = _call_retry_policy(self.retry, ctx, None, err)
            return decision.retry or decision.nack

        event_ctx = self.build_context(ctx, topic, msg)
        if self.client_provider is not None:
            try:
                evt.client = _resolve_client_provider(self.client_provider)(
                    event_ctx, evt
                )
            except Exception as err:
                _log_printf(self.logger, "client init failed: %s", err)
                self.update_event_log_status(
                    event_ctx, log_id, EVENT_LOG_STATUS_FAILED, err
                )
                self.notify_error(event_ctx, evt, err)
                decision = _call_retry_policy(self.retry, event_ctx, evt, err)
                return decision.retry or decision.nack

        req_id = evt.metadata.get(METADATA_KEY_REQUEST_ID, "")
        if req_id:
            _log_printf(
                self.logger,
                "request_id=%s topic=%s provider=%s type=%s",
                req_id,
                evt.topic,
                evt.provider,
                evt.type,
            )

        self.notify_message_start(event_ctx, evt)

        handler = self.topic_handlers.get(topic) or self.type_handlers.get(evt.type)
        if handler is None:
            _log_printf(self.logger, "no handler for topic=%s type=%s", topic, evt.type)
            self.notify_message_finish(event_ctx, evt, None)
            self.update_event_log_status(
                event_ctx, log_id, EVENT_LOG_STATUS_SUCCESS, None
            )
            return False

        wrapped = self.wrap(handler)
        last_err: Optional[Exception] = None
        attempts = self.retry_count + 1
        for _ in range(attempts):
            try:
                wrapped(event_ctx, evt)
                last_err = None
                break
            except Exception as err:
                last_err = err
        if last_err is None:
            self.notify_message_finish(event_ctx, evt, None)
            self.update_event_log_status(
                event_ctx, log_id, EVENT_LOG_STATUS_SUCCESS, None
            )
            return False
        self.notify_message_finish(event_ctx, evt, last_err)
        self.notify_error(event_ctx, evt, last_err)
        self.update_event_log_status(
            event_ctx, log_id, EVENT_LOG_STATUS_FAILED, last_err
        )
        decision = _call_retry_policy(self.retry, event_ctx, evt, last_err)
        return decision.retry or decision.nack

    def wrap(self, handler: ContextualHandler) -> ContextualHandler:
        wrapped = handler
        for mw in reversed(self.middleware):
            wrapped = mw(wrapped)
        return wrapped

    def build_context(
        self, base: WorkerContext, topic: str, msg: RelaybusMessage
    ) -> WorkerContext:
        metadata = msg.metadata or {}
        metadata_tenant = str(metadata.get(METADATA_KEY_TENANT_ID, "")).strip()
        tenant_id = metadata_tenant or (base.tenant_id or "").strip()
        return WorkerContext(
            tenant_id=tenant_id,
            signal=base.signal,
            topic=topic,
            request_id=metadata.get(METADATA_KEY_REQUEST_ID, ""),
            log_id=metadata.get(METADATA_KEY_LOG_ID, ""),
        )

    def validate_topics(self, ctx: WorkerContext) -> None:
        rules = self.rules_client().list_rules(ctx)
        if not rules:
            raise ValueError("no rules available from api")
        allowed_topics: Set[str] = set()
        allowed_by_driver: Dict[str, Set[str]] = {}
        for record in rules:
            for topic in record.emit:
                trimmed = (topic or "").strip()
                if not trimmed:
                    continue
                allowed_topics.add(trimmed)
                driver_id = (record.driver_id or "").strip()
                if not driver_id:
                    continue
                allowed_by_driver.setdefault(driver_id, set()).add(trimmed)
        if not allowed_topics:
            raise ValueError("no topics available from rules")

        topics = _unique(self.topics)
        if self.subscriber is not None:
            for topic in topics:
                if topic not in allowed_topics:
                    raise ValueError(f"unknown topic: {topic}")
            return

        for topic in topics:
            driver_id = (self.topic_drivers.get(topic) or "").strip()
            if not driver_id:
                driver_id = (self.default_driver_id or "").strip()
            if not driver_id:
                raise ValueError(f"driver id is required for topic: {topic}")
            allowed = allowed_by_driver.get(driver_id)
            if not allowed:
                raise ValueError(f"driver not configured on any rule: {driver_id}")
            if topic not in allowed:
                raise ValueError(f"topic {topic} not configured for driver {driver_id}")

    def prepare_rule_subscriptions(self, ctx: WorkerContext) -> None:
        if not self.rule_handlers:
            return
        client = self.rules_client()
        for rule_id, handler in self.rule_handlers.items():
            record = client.get_rule(rule_id, ctx)
            if not record.emit:
                raise ValueError(f"rule {rule_id} has no emit topic")
            topic = (record.emit[0] or "").strip()
            if not topic:
                raise ValueError(f"rule {rule_id} emit topic empty")
            driver_id = (record.driver_id or "").strip()
            if not driver_id:
                raise ValueError(f"rule {rule_id} driver_id is required")
            if topic in self.topic_handlers:
                _log_printf(
                    self.logger,
                    "overwriting handler for topic=%s due to rule=%s",
                    topic,
                    rule_id,
                )
            self.topic_handlers[topic] = handler
            self.topic_drivers[topic] = driver_id
            self.topics.append(topic)

    def notify_start(self, ctx: WorkerContext) -> None:
        for listener in self.listeners:
            listener.OnStart(ctx)

    def notify_exit(self, ctx: WorkerContext) -> None:
        for listener in self.listeners:
            listener.OnExit(ctx)

    def notify_message_start(self, ctx: WorkerContext, evt: Event) -> None:
        for listener in self.listeners:
            listener.OnMessageStart(ctx, evt)

    def notify_message_finish(
        self, ctx: WorkerContext, evt: Event, err: Optional[Exception]
    ) -> None:
        for listener in self.listeners:
            listener.OnMessageFinish(ctx, evt, err)

    def notify_error(
        self, ctx: WorkerContext, evt: Optional[Event], err: Exception
    ) -> None:
        for listener in self.listeners:
            listener.OnError(ctx, evt, err)

    def drivers_client(self) -> DriversClient:
        return DriversClient(self.api_client_options())

    def rules_client(self) -> RulesClient:
        return RulesClient(self.api_client_options())

    def event_logs_client(self) -> EventLogsClient:
        return EventLogsClient(self.api_client_options())

    def api_client_options(self) -> APIClientOptions:
        return APIClientOptions(
            base_url=self.endpoint,
            api_key=self.api_key,
            oauth2_config=self.oauth2_config,
            tenant_id=self.tenant_id,
        )

    def update_event_log_status(
        self,
        ctx: WorkerContext,
        log_id: str,
        status: str,
        err: Optional[Exception],
    ) -> None:
        if not log_id:
            return
        try:
            self.event_logs_client().update_status(log_id, status, str(err or ""), ctx)
        except Exception as update_err:
            _log_printf(self.logger, "event log update failed: %s", update_err)

    def bind_client_provider(self) -> None:
        if self.client_provider is None:
            return
        provider = self.client_provider
        opts = self.api_client_options()
        if hasattr(provider, "bind_api_client"):
            provider.bind_api_client(opts)
        elif hasattr(provider, "BindAPIClient"):
            provider.BindAPIClient(opts)


def New(*options: WorkerOption) -> Worker:
    return Worker.new(*options)


def WithSubscriber(subscriber: Subscriber) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(subscriber=subscriber))


def WithTopics(*topics: str) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(topics=list(topics)))


def WithConcurrency(concurrency: int) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(concurrency=concurrency))


def WithCodec(codec: Codec) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(codec=codec))


def WithMiddleware(*middleware: Middleware) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(middleware=list(middleware)))


def WithRetry(retry: RetryPolicy) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(retry=retry))


def WithRetryCount(retry_count: int) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(retry_count=retry_count))


def WithLogger(logger: Logger) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(logger=logger))


def WithClientProvider(client_provider: ClientProvider) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(client_provider=client_provider))


def WithListener(listener: Listener) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(listeners=[listener]))


def WithEndpoint(endpoint: str) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(endpoint=endpoint))


def WithAPIKey(api_key: str) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(api_key=api_key))


def WithOAuth2Config(oauth2_config: OAuth2Config) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(oauth2_config=oauth2_config))


def WithTenant(tenant_id: str) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(tenant_id=tenant_id))


def WithDefaultDriver(driver_id: str) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(default_driver_id=driver_id))


def WithValidateTopics(validate: bool) -> WorkerOption:
    return lambda wk: wk.apply(WorkerOptions(validate_topics=validate))


class _StdLogger:
    def printf(self, fmt: str, *args: Any) -> None:
        if args:
            print(f"relaymesh/worker {fmt % args}")
        else:
            print(f"relaymesh/worker {fmt}")

    def Printf(self, fmt: str, *args: Any) -> None:
        self.printf(fmt, *args)


def _log_printf(logger: Logger, fmt: str, *args: Any) -> None:
    if hasattr(logger, "Printf"):
        logger.Printf(fmt, *args)
        return
    if hasattr(logger, "printf"):
        logger.printf(fmt, *args)
        return
    if args:
        print(f"relaymesh/worker {fmt % args}")
    else:
        print(f"relaymesh/worker {fmt}")


def _resolve_client_provider(
    provider: ClientProvider,
) -> Callable[[WorkerContext, Event], Any]:
    if hasattr(provider, "client"):
        return provider.client  # type: ignore[return-value]
    if hasattr(provider, "Client"):
        return provider.Client  # type: ignore[return-value]
    return lambda _ctx, _evt: None


def _to_context_handler(handler: Handler) -> ContextualHandler:
    try:
        sig = inspect.signature(handler)
        params = [
            param
            for param in sig.parameters.values()
            if param.kind
            in (
                inspect.Parameter.POSITIONAL_ONLY,
                inspect.Parameter.POSITIONAL_OR_KEYWORD,
            )
        ]
        has_varargs = any(
            param.kind == inspect.Parameter.VAR_POSITIONAL
            for param in sig.parameters.values()
        )
        if has_varargs or len(params) >= 2:
            return handler  # type: ignore[return-value]
    except (ValueError, TypeError):
        return lambda _ctx, evt: handler(evt)  # type: ignore[misc]

    return lambda _ctx, evt: handler(evt)  # type: ignore[misc]


def _call_retry_policy(
    policy: RetryPolicy, ctx: WorkerContext, evt: Optional[Event], err: Exception
):
    if hasattr(policy, "OnError"):
        return normalize_retry_decision(policy.OnError(ctx, evt, err))
    return normalize_retry_decision(policy.on_error(ctx, evt, err))


def _should_requeue(msg: RelaybusMessage) -> bool:
    driver = (msg.metadata or {}).get(METADATA_KEY_DRIVER, "").lower()
    return driver == "amqp"


def _unique(values: Sequence[str]) -> List[str]:
    seen: Set[str] = set()
    out: List[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        out.append(value)
    return out
