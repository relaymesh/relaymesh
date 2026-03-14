from dataclasses import dataclass, field
from typing import Dict, Optional


@dataclass
class RelaybusMessage:
    topic: str
    payload: bytes
    metadata: Dict[str, str] = field(default_factory=dict)
    content_type: str = ""


def coerce_message(msg: object) -> RelaybusMessage:
    topic = getattr(msg, "topic", "") or ""
    payload = getattr(msg, "payload", b"") or b""
    metadata = getattr(msg, "metadata", None) or getattr(msg, "meta", None)
    content_type = getattr(msg, "content_type", "") or ""
    if metadata is None:
        metadata = {}
    return RelaybusMessage(
        topic=str(topic),
        payload=payload,
        metadata=dict(metadata),
        content_type=str(content_type),
    )
