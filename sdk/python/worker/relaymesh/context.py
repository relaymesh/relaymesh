from dataclasses import dataclass
from threading import Event as ThreadEvent
from typing import Optional


@dataclass
class WorkerContext:
    tenant_id: str = ""
    signal: Optional[ThreadEvent] = None
    topic: str = ""
    request_id: str = ""
    log_id: str = ""
