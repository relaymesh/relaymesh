import datetime
import threading
from dataclasses import dataclass
from typing import Optional

from .api import APIClientOptions, SCMClientsClient, resolve_endpoint
from .client import ClientProvider
from .metadata import METADATA_KEY_INSTALLATION_ID, METADATA_KEY_PROVIDER_INSTANCE_KEY
from .oauth2 import OAuth2Config, resolve_oauth2_config
from .scm_clients import (
    AtlassianClientFromEvent,
    BitbucketClientFromEvent,
    GitHubClientFromEvent,
    GitLabClientFromEvent,
    JiraClientFromEvent,
    SlackClientFromEvent,
    new_provider_client,
)

DEFAULT_SCM_CACHE_SIZE = 10
DEFAULT_SCM_CACHE_SKEW_SECONDS = 30


@dataclass
class RemoteSCMClientProviderOptions:
    endpoint: str = ""
    api_key: str = ""
    oauth2_config: Optional[OAuth2Config] = None
    cache_size: int = DEFAULT_SCM_CACHE_SIZE
    cache_skew_seconds: int = DEFAULT_SCM_CACHE_SKEW_SECONDS
    timeout: float = 10.0


class RemoteSCMClientProvider(ClientProvider):
    def __init__(self, opts: Optional[RemoteSCMClientProviderOptions] = None) -> None:
        options = opts or RemoteSCMClientProviderOptions()
        self.endpoint = resolve_endpoint(options.endpoint)
        self.api_key = (options.api_key or "").strip()
        self.oauth2_config = resolve_oauth2_config(options.oauth2_config)
        self.cache = _SCMClientCache(max(1, options.cache_size))
        self.cache_skew_seconds = max(0, int(options.cache_skew_seconds))
        self.timeout = (
            options.timeout if options.timeout and options.timeout > 0 else 10.0
        )

    def bind_api_client(self, opts: APIClientOptions) -> None:
        if opts.base_url:
            self.endpoint = resolve_endpoint(opts.base_url)
        if opts.api_key:
            self.api_key = opts.api_key
        if opts.oauth2_config:
            self.oauth2_config = opts.oauth2_config
        if opts.timeout:
            self.timeout = opts.timeout

    def BindAPIClient(self, opts: APIClientOptions) -> None:
        self.bind_api_client(opts)

    def client(self, ctx, evt) -> object:
        return self.Client(ctx, evt)

    def Client(self, ctx, evt) -> object:
        if evt is None:
            raise ValueError("event is required")
        provider = (evt.provider or "").strip()
        if not provider:
            raise ValueError("provider is required")
        installation_id = (
            (evt.metadata or {}).get(METADATA_KEY_INSTALLATION_ID, "").strip()
        )
        if not installation_id:
            raise ValueError("installation_id missing from metadata")
        instance_key = (
            (evt.metadata or {}).get(METADATA_KEY_PROVIDER_INSTANCE_KEY, "").strip()
        )
        cache_key = "|".join([provider, installation_id, instance_key])
        cached = self.cache.get(cache_key, self.cache_skew_seconds)
        if cached is not None:
            return cached

        client = self._fetch_client(ctx, provider, installation_id, instance_key)
        if client.client is not None:
            self.cache.add(cache_key, client.client, client.expires_at)
        return client.client

    def _fetch_client(
        self, ctx, provider: str, installation_id: str, instance_key: str
    ) -> "_SCMClientResult":
        api_opts = APIClientOptions(
            base_url=self.endpoint,
            api_key=self.api_key,
            oauth2_config=self.oauth2_config,
            tenant_id=getattr(ctx, "tenant_id", ""),
            timeout=self.timeout,
        )
        record = SCMClientsClient(api_opts).get_scm_client(
            provider, installation_id, instance_key, ctx
        )
        if not record.access_token:
            raise ValueError("scm access token missing")
        created = new_provider_client(
            record.provider, record.access_token, record.api_base_url
        )
        return _SCMClientResult(client=created, expires_at=record.expires_at)


def NewRemoteSCMClientProvider(
    opts: Optional[RemoteSCMClientProviderOptions] = None,
) -> RemoteSCMClientProvider:
    return RemoteSCMClientProvider(opts)


def GitHubClient(evt: object):
    return GitHubClientFromEvent(evt)


def GitLabClient(evt: object):
    return GitLabClientFromEvent(evt)


def BitbucketClient(evt: object):
    return BitbucketClientFromEvent(evt)


def SlackClient(evt: object):
    return SlackClientFromEvent(evt)


def JiraClient(evt: object):
    return JiraClientFromEvent(evt)


def AtlassianClient(evt: object):
    return AtlassianClientFromEvent(evt)


@dataclass
class _SCMClientResult:
    client: Optional[object]
    expires_at: Optional[datetime.datetime]


class _SCMClientCache:
    def __init__(self, size: int) -> None:
        self.size = max(1, size)
        self.lock = threading.Lock()
        self.items: "dict[str, _SCMCacheEntry]" = {}
        self.order: "list[str]" = []

    def get(self, key: str, skew_seconds: int) -> Optional[object]:
        if not key:
            return None
        with self.lock:
            entry = self.items.get(key)
            if entry is None:
                return None
            if _expired(entry.expires_at, skew_seconds):
                self._delete(key)
                return None
            self._bump(key)
            return entry.client

    def add(
        self, key: str, client: object, expires_at: Optional[datetime.datetime]
    ) -> None:
        if not key or client is None:
            return
        with self.lock:
            if key in self.items:
                self.items[key] = _SCMCacheEntry(client=client, expires_at=expires_at)
                self._bump(key)
                return
            self.items[key] = _SCMCacheEntry(client=client, expires_at=expires_at)
            self.order.insert(0, key)
            while len(self.order) > self.size:
                old_key = self.order.pop()
                self.items.pop(old_key, None)

    def _bump(self, key: str) -> None:
        if key in self.order:
            self.order.remove(key)
        self.order.insert(0, key)

    def _delete(self, key: str) -> None:
        self.items.pop(key, None)
        if key in self.order:
            self.order.remove(key)


@dataclass
class _SCMCacheEntry:
    client: object
    expires_at: Optional[datetime.datetime]


def _expired(expires_at: Optional[datetime.datetime], skew_seconds: int) -> bool:
    if expires_at is None:
        return False
    now = datetime.datetime.now(tz=datetime.timezone.utc)
    skew = datetime.timedelta(seconds=max(0, skew_seconds))
    return now >= (expires_at - skew)
