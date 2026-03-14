from dataclasses import dataclass, field
from typing import List


@dataclass
class KafkaConfig:
    brokers: List[str] = field(default_factory=list)
    broker: str = ""
    group_id: str = ""
    topic_prefix: str = ""
    max_messages: int = 0


@dataclass
class NatsConfig:
    url: str = ""
    subject_prefix: str = ""
    max_messages: int = 0


@dataclass
class AmqpConfig:
    url: str = ""
    exchange: str = ""
    routing_key_template: str = ""
    queue: str = ""
    auto_ack: bool = False
    max_messages: int = 0


@dataclass
class SubscriberConfig:
    driver: str = ""
    drivers: List[str] = field(default_factory=list)
    kafka: KafkaConfig = field(default_factory=KafkaConfig)
    nats: NatsConfig = field(default_factory=NatsConfig)
    amqp: AmqpConfig = field(default_factory=AmqpConfig)
