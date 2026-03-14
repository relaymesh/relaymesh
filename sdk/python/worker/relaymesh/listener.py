from typing import Optional

from .context import WorkerContext
from .event import Event


class Listener:
    def on_start(self, ctx: WorkerContext) -> None:
        return None

    def on_exit(self, ctx: WorkerContext) -> None:
        return None

    def on_message_start(self, ctx: WorkerContext, evt: Event) -> None:
        return None

    def on_message_finish(
        self, ctx: WorkerContext, evt: Event, err: Optional[Exception] = None
    ) -> None:
        return None

    def on_error(
        self, ctx: WorkerContext, evt: Optional[Event], err: Exception
    ) -> None:
        return None

    def OnStart(self, ctx: WorkerContext) -> None:
        return self.on_start(ctx)

    def OnExit(self, ctx: WorkerContext) -> None:
        return self.on_exit(ctx)

    def OnMessageStart(self, ctx: WorkerContext, evt: Event) -> None:
        return self.on_message_start(ctx, evt)

    def OnMessageFinish(
        self, ctx: WorkerContext, evt: Event, err: Optional[Exception] = None
    ) -> None:
        return self.on_message_finish(ctx, evt, err)

    def OnError(self, ctx: WorkerContext, evt: Optional[Event], err: Exception) -> None:
        return self.on_error(ctx, evt, err)
