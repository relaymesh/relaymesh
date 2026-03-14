import json
import threading
from dataclasses import asdict
from typing import Callable, Dict, List, Optional

from .config import AmqpConfig, KafkaConfig, NatsConfig, SubscriberConfig
from .metadata import METADATA_KEY_DRIVER
from .types import RelaybusMessage, coerce_message

MessageHandler = Callable[[RelaybusMessage], Optional[bool]]


class Subscriber:
    def start(self, topic: str, handler: MessageHandler) -> None:
        raise NotImplementedError

    def close(self) -> None:
        return None


def subscriber_config_from_driver(driver: str, raw: str) -> SubscriberConfig:
    driver = (driver or "").strip().lower()
    if not driver:
        raise ValueError("driver is required")
    cfg = SubscriberConfig(driver=driver)
    apply_driver_config(cfg, driver, raw)
    return cfg


def apply_driver_config(cfg: SubscriberConfig, name: str, raw: str) -> None:
    if cfg is None:
        raise ValueError("config is required")
    payload = (raw or "").strip()
    if not payload:
        return
    data = json.loads(payload)
    if not isinstance(data, dict):
        return
    name = (name or "").strip().lower()
    if name == "amqp":
        apply_amqp_config(cfg.amqp, data)
        return
    if name == "nats":
        apply_nats_config(cfg.nats, data)
        return
    if name == "kafka":
        apply_kafka_config(cfg.kafka, data)
        return
    raise ValueError(f"unsupported driver: {name}")


def apply_amqp_config(cfg: AmqpConfig, data: Dict[str, object]) -> None:
    cfg.url = read_string(data, "url")
    cfg.exchange = read_string(data, "exchange")
    cfg.routing_key_template = read_string(
        data, "routing_key_template", "routingKeyTemplate"
    )
    cfg.queue = read_string(data, "queue")
    cfg.auto_ack = read_bool(data, "auto_ack", "autoAck")
    cfg.max_messages = read_int(data, "max_messages", "maxMessages")


def apply_nats_config(cfg: NatsConfig, data: Dict[str, object]) -> None:
    cfg.url = read_string(data, "url")
    cfg.subject_prefix = read_string(data, "subject_prefix", "subjectPrefix")
    cfg.max_messages = read_int(data, "max_messages", "maxMessages")


def apply_kafka_config(cfg: KafkaConfig, data: Dict[str, object]) -> None:
    brokers = read_string_list(data, "brokers")
    broker = read_string(data, "broker")
    if not brokers and broker:
        brokers = [broker]
    cfg.brokers = brokers
    cfg.broker = broker
    cfg.group_id = read_string(data, "group_id", "groupId")
    cfg.topic_prefix = read_string(data, "topic_prefix", "topicPrefix")
    cfg.max_messages = read_int(data, "max_messages", "maxMessages")


def build_subscriber(cfg: SubscriberConfig) -> Subscriber:
    drivers = unique_strings(cfg.drivers)
    if cfg.driver:
        drivers.append(cfg.driver)
        drivers = unique_strings(drivers)
    if not drivers:
        raise ValueError("at least one driver is required")
    if len(drivers) == 1:
        return _RelaybusSubscriber(cfg, drivers[0])
    subs = [_RelaybusSubscriber(cfg, driver) for driver in drivers]
    return _MultiSubscriber(subs)


def new_from_config(cfg: SubscriberConfig, *opts) -> "Worker":
    from .worker import Worker, WithSubscriber

    sub = build_subscriber(cfg)
    options = list(opts) + [WithSubscriber(sub)]
    return Worker.new(*options)


class _RelaybusSubscriber(Subscriber):
    def __init__(self, cfg: SubscriberConfig, driver: str) -> None:
        self.driver = (driver or "").strip().lower()
        self.cfg = cfg
        self._inner = None

    def start(self, topic: str, handler: MessageHandler) -> None:
        if not handler:
            raise ValueError("handler is required")
        if self.driver == "amqp":
            self._start_amqp(topic, handler)
            return
        if self.driver == "nats":
            self._start_nats(topic, handler)
            return
        if self.driver == "kafka":
            self._start_kafka(topic, handler)
            return
        raise ValueError(f"unsupported subscriber driver: {self.driver}")

    def close(self) -> None:
        if self._inner is not None:
            try:
                self._inner.close()
            except Exception:
                return None

    def _start_amqp(self, topic: str, handler: MessageHandler) -> None:
        if not self.cfg.amqp.url:
            raise ValueError("amqp url is required")
        from relaybus_amqp import AmqpSubscriber, AmqpSubscriberConnectConfig

        def on_message(msg: object) -> None:
            relay_msg = coerce_message(msg)
            if self.driver and METADATA_KEY_DRIVER not in relay_msg.metadata:
                relay_msg.metadata[METADATA_KEY_DRIVER] = self.driver
            result = handler(relay_msg)
            if result:
                raise RuntimeError("message nack requested")

        self._inner = AmqpSubscriber.connect(
            AmqpSubscriberConnectConfig(
                url=self.cfg.amqp.url,
                exchange=self.cfg.amqp.exchange,
                routing_key_template=self.cfg.amqp.routing_key_template,
                queue=self.cfg.amqp.queue,
                on_message=on_message,
            )
        )
        while True:
            try:
                self._inner.start(topic)
            except TimeoutError:
                continue
            except Exception:
                break

    def _start_nats(self, topic: str, handler: MessageHandler) -> None:
        if not self.cfg.nats.url:
            raise ValueError("nats url is required")
        from relaybus_nats import NatsSubscriber, NatsSubscriberConnectConfig

        def on_message(msg: object) -> None:
            relay_msg = coerce_message(msg)
            if self.driver and METADATA_KEY_DRIVER not in relay_msg.metadata:
                relay_msg.metadata[METADATA_KEY_DRIVER] = self.driver
            handler(relay_msg)

        self._inner = NatsSubscriber.connect(
            NatsSubscriberConnectConfig(
                url=self.cfg.nats.url,
                subject_prefix=self.cfg.nats.subject_prefix,
                on_message=on_message,
            )
        )
        while True:
            try:
                self._inner.start(topic)
            except TimeoutError:
                continue
            except Exception:
                break

    def _start_kafka(self, topic: str, handler: MessageHandler) -> None:
        from relaybus_kafka import KafkaSubscriber, KafkaSubscriberConnectConfig

        brokers = list(self.cfg.kafka.brokers or [])
        if not brokers and self.cfg.kafka.broker:
            brokers = [self.cfg.kafka.broker]
        if not brokers:
            raise ValueError("kafka brokers are required")

        def on_message(msg: object) -> None:
            relay_msg = coerce_message(msg)
            if self.driver and METADATA_KEY_DRIVER not in relay_msg.metadata:
                relay_msg.metadata[METADATA_KEY_DRIVER] = self.driver
            handler(relay_msg)

        self._inner = KafkaSubscriber.connect(
            KafkaSubscriberConnectConfig(
                brokers=brokers,
                group_id=self.cfg.kafka.group_id,
                max_messages=self.cfg.kafka.max_messages,
                on_message=on_message,
            )
        )
        kafka_topic = topic
        if self.cfg.kafka.topic_prefix:
            kafka_topic = f"{self.cfg.kafka.topic_prefix}{topic}"
        while True:
            try:
                self._inner.start(kafka_topic)
            except TimeoutError:
                continue
            except Exception:
                break


class _MultiSubscriber(Subscriber):
    def __init__(self, subs: List[_RelaybusSubscriber]) -> None:
        self.subs = subs
        self.threads: List[threading.Thread] = []

    def start(self, topic: str, handler: MessageHandler) -> None:
        if not self.subs:
            raise ValueError("no subscribers configured")
        if not handler:
            raise ValueError("handler is required")

        self.threads = []
        for sub in self.subs:
            t = threading.Thread(target=sub.start, args=(topic, handler), daemon=True)
            t.start()
            self.threads.append(t)

        for t in self.threads:
            t.join()

    def close(self) -> None:
        for sub in self.subs:
            sub.close()


def unique_strings(values: List[str]) -> List[str]:
    seen = set()
    out: List[str] = []
    for value in values:
        normalized = (value or "").strip().lower()
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        out.append(normalized)
    return out


def read_string(data: Dict[str, object], *keys: str) -> str:
    for key in keys:
        value = data.get(key)
        if isinstance(value, str):
            return value
    return ""


def read_bool(data: Dict[str, object], *keys: str) -> bool:
    for key in keys:
        value = data.get(key)
        if isinstance(value, bool):
            return value
    return False


def read_int(data: Dict[str, object], *keys: str) -> int:
    for key in keys:
        value = data.get(key)
        if isinstance(value, int):
            return value
    return 0


def read_string_list(data: Dict[str, object], *keys: str) -> List[str]:
    for key in keys:
        value = data.get(key)
        if isinstance(value, list):
            return [
                str(item) for item in value if isinstance(item, str) or item is not None
            ]
    return []
