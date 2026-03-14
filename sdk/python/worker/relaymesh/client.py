from typing import Any, Callable, Protocol

from .api import APIClientOptions
from .context import WorkerContext
from .event import Event


class ClientProvider(Protocol):
    def client(self, ctx: WorkerContext, evt: Event) -> Any: ...

    def Client(self, ctx: WorkerContext, evt: Event) -> Any: ...

    def bind_api_client(self, opts: APIClientOptions) -> None: ...

    def BindAPIClient(self, opts: APIClientOptions) -> None: ...


def client_provider_func(fn: Callable[[WorkerContext, Event], Any]) -> ClientProvider:
    class _Provider:
        def client(self, ctx: WorkerContext, evt: Event) -> Any:
            return fn(ctx, evt)

        def Client(self, ctx: WorkerContext, evt: Event) -> Any:
            return fn(ctx, evt)

        def bind_api_client(self, opts: APIClientOptions) -> None:
            return None

        def BindAPIClient(self, opts: APIClientOptions) -> None:
            return None

    provider: ClientProvider = _Provider()
    return provider


def ClientProviderFunc(fn: Callable[[WorkerContext, Event], Any]) -> ClientProvider:
    return client_provider_func(fn)
