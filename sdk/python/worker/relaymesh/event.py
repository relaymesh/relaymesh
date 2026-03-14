from dataclasses import dataclass, field
from typing import Any, Dict, Optional


@dataclass
class Event:
    provider: str
    type: str
    topic: str
    metadata: Dict[str, str] = field(default_factory=dict)
    payload: bytes = b""
    normalized: Optional[Dict[str, Any]] = None
    request_id: str = ""
    installation_id: str = ""
    log_id: str = ""
    client: Optional[Any] = None
