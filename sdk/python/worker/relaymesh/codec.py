import json
from typing import Any, Dict, Optional

from cloud.v1 import githooks_pb2

from .event import Event
from .metadata import (
    METADATA_KEY_EVENT,
    METADATA_KEY_INSTALLATION_ID,
    METADATA_KEY_LOG_ID,
    METADATA_KEY_PROVIDER,
    METADATA_KEY_REQUEST_ID,
)
from .types import RelaybusMessage


class Codec:
    def decode(self, topic: Optional[str], msg: RelaybusMessage) -> Event:
        raise NotImplementedError


class DefaultCodec(Codec):
    def decode(self, topic: Optional[str], msg: RelaybusMessage) -> Event:
        if msg is None or msg.payload is None:
            raise ValueError("message payload is required")

        provider = ""
        event_name = ""
        raw_payload = msg.payload
        normalized: Optional[Dict[str, Any]] = None

        try:
            env = githooks_pb2.EventPayload()
            env.ParseFromString(msg.payload)
            provider = env.provider
            event_name = env.name
            raw_payload = env.payload or b""
            normalized = _parse_json_object(raw_payload)
        except Exception:
            legacy = _parse_json_value(msg.payload)
            if isinstance(legacy, dict):
                provider = str(legacy.get("provider", provider) or provider)
                event_name = str(legacy.get("name", event_name) or event_name)
                data = legacy.get("data")
                if isinstance(data, dict):
                    normalized = data
            if normalized is None:
                normalized = _parse_json_object(raw_payload)

        metadata = dict(msg.metadata or {})
        if not provider:
            provider = metadata.get(METADATA_KEY_PROVIDER, "")
        if not event_name:
            event_name = metadata.get(METADATA_KEY_EVENT, "")

        return Event(
            provider=provider,
            type=event_name,
            topic=_resolve_topic(topic, msg),
            metadata=metadata,
            payload=raw_payload,
            normalized=normalized,
            request_id=metadata.get(METADATA_KEY_REQUEST_ID, ""),
            installation_id=metadata.get(METADATA_KEY_INSTALLATION_ID, ""),
            log_id=metadata.get(METADATA_KEY_LOG_ID, ""),
        )


def _resolve_topic(topic: Optional[str], msg: RelaybusMessage) -> str:
    trimmed = (topic or "").strip()
    if trimmed:
        return trimmed
    return str(msg.topic or "")


def _parse_json_object(data: bytes) -> Optional[Dict[str, Any]]:
    value = _parse_json_value(data)
    if isinstance(value, dict):
        return value
    return None


def _parse_json_value(data: bytes) -> Optional[Any]:
    if not data:
        return None
    try:
        return json.loads(data.decode("utf-8"))
    except Exception:
        return None
