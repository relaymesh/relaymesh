import unittest

from relaymesh.context import WorkerContext
from relaymesh.codec import Codec
from relaymesh.event import Event
from relaymesh.event_log_status import (
    EVENT_LOG_STATUS_FAILED,
    EVENT_LOG_STATUS_SUCCESS,
)
from relaymesh.listener import Listener
from relaymesh.retry import RetryPolicy
from relaymesh.retry import RetryDecision
from relaymesh.types import RelaybusMessage
from relaymesh.worker import Worker, WorkerOptions


class TestCodec(Codec):
    def __init__(self, event=None, err=None):
        self._event = event
        self._err = err

    def decode(self, topic, msg):
        if self._err is not None:
            raise self._err
        if self._event is None:
            raise ValueError("event is required for TestCodec")
        return self._event


class TestRetryPolicy(RetryPolicy):
    def __init__(self, decision):
        self.decision = decision
        self.calls = 0
        self.last_evt = None
        self.last_err = None

    def on_error(self, ctx, evt, err):
        self.calls += 1
        self.last_evt = evt
        self.last_err = err
        return self.decision


class TestListener(Listener):
    def __init__(self):
        self.finish_calls = []
        self.error_calls = []

    def OnStart(self, ctx):
        return None

    def OnExit(self, ctx):
        return None

    def OnMessageStart(self, ctx, evt):
        return None

    def OnMessageFinish(self, ctx, evt, err=None):
        self.finish_calls.append(err)

    def OnError(self, ctx, evt, err):
        self.error_calls.append((evt, err))


class TestWorker(Worker):
    def __init__(self, opts, status_calls):
        super().__init__(opts)
        self._status_calls = status_calls

    def update_event_log_status(self, ctx, log_id, status, err):
        self._status_calls.append((log_id, status, str(err or "")))


class WorkerHandleMessageTests(unittest.TestCase):
    def test_handle_message_success_updates_success_status(self):
        event = Event(
            provider="github",
            type="push",
            topic="topic",
            metadata={},
            payload=b"{}",
        )
        retry = TestRetryPolicy(RetryDecision(retry=False, nack=True))
        listener = TestListener()
        status_calls = []
        worker = TestWorker(
            WorkerOptions(
                codec=TestCodec(event=event),
                retry=retry,
                listeners=[listener],
            ),
            status_calls,
        )
        worker.topic_handlers["topic"] = lambda ctx, evt: None

        should_nack = worker.handle_message(
            WorkerContext(tenant_id="acme"),
            "topic",
            RelaybusMessage(topic="topic", payload=b"{}", metadata={"log_id": "log-1"}),
        )

        self.assertFalse(should_nack)
        self.assertEqual(retry.calls, 0)
        self.assertEqual(listener.finish_calls, [None])
        self.assertEqual(status_calls, [("log-1", EVENT_LOG_STATUS_SUCCESS, "")])

    def test_handle_message_handler_error_updates_failed_status(self):
        event = Event(
            provider="github",
            type="push",
            topic="topic",
            metadata={},
            payload=b"{}",
        )
        retry = TestRetryPolicy(RetryDecision(retry=False, nack=True))
        listener = TestListener()
        status_calls = []
        worker = TestWorker(
            WorkerOptions(
                codec=TestCodec(event=event),
                retry=retry,
                listeners=[listener],
            ),
            status_calls,
        )

        def fail_handler(ctx, evt):
            raise RuntimeError("handler failed")

        worker.topic_handlers["topic"] = fail_handler

        should_nack = worker.handle_message(
            WorkerContext(tenant_id="acme"),
            "topic",
            RelaybusMessage(topic="topic", payload=b"{}", metadata={"log_id": "log-2"}),
        )

        self.assertTrue(should_nack)
        self.assertEqual(retry.calls, 1)
        self.assertIsNotNone(retry.last_evt)
        self.assertEqual(str(retry.last_err), "handler failed")
        self.assertEqual(len(listener.error_calls), 1)
        self.assertIsNotNone(listener.error_calls[0][0])
        self.assertEqual(str(listener.error_calls[0][1]), "handler failed")
        self.assertEqual(
            status_calls,
            [("log-2", EVENT_LOG_STATUS_FAILED, "handler failed")],
        )

    def test_handle_message_decode_error_uses_nil_event(self):
        retry = TestRetryPolicy(RetryDecision(retry=True, nack=False))
        listener = TestListener()
        status_calls = []
        worker = TestWorker(
            WorkerOptions(
                codec=TestCodec(err=ValueError("decode failed")),
                retry=retry,
                listeners=[listener],
            ),
            status_calls,
        )

        should_nack = worker.handle_message(
            WorkerContext(tenant_id="acme"),
            "topic",
            RelaybusMessage(topic="topic", payload=b"{}", metadata={"log_id": "log-3"}),
        )

        self.assertTrue(should_nack)
        self.assertEqual(retry.calls, 1)
        self.assertIsNone(retry.last_evt)
        self.assertEqual(str(retry.last_err), "decode failed")
        self.assertEqual(len(listener.error_calls), 1)
        self.assertIsNone(listener.error_calls[0][0])
        self.assertEqual(str(listener.error_calls[0][1]), "decode failed")
        self.assertEqual(
            status_calls,
            [("log-3", EVENT_LOG_STATUS_FAILED, "decode failed")],
        )


if __name__ == "__main__":
    unittest.main()
