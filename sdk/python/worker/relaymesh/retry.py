from dataclasses import dataclass
from typing import Optional

from .context import WorkerContext
from .event import Event


@dataclass
class RetryDecision:
    retry: bool = False
    nack: bool = True


class RetryPolicy:
    def on_error(
        self, ctx: WorkerContext, evt: Optional[Event], err: Exception
    ) -> RetryDecision:
        return RetryDecision()

    def OnError(
        self, ctx: WorkerContext, evt: Optional[Event], err: Exception
    ) -> RetryDecision:
        return self.on_error(ctx, evt, err)


class NoRetry(RetryPolicy):
    def on_error(
        self, ctx: WorkerContext, evt: Optional[Event], err: Exception
    ) -> RetryDecision:
        return RetryDecision(retry=False, nack=True)


def normalize_retry_decision(value: object) -> RetryDecision:
    if isinstance(value, RetryDecision):
        return value
    if isinstance(value, dict):
        retry = bool(value.get("retry", value.get("Retry", False)))
        nack = bool(value.get("nack", value.get("Nack", True)))
        return RetryDecision(retry=retry, nack=nack)
    return RetryDecision()
