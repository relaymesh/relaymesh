package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	relaymessage "github.com/relaymesh/relaybus/sdk/core/go/message"
	"google.golang.org/protobuf/proto"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
)

// stubPublisher is a mock Publisher for testing.
type stubPublisher struct {
	published int
	lastTopic string
}

func (s *stubPublisher) Publish(ctx context.Context, topic string, event Event) error {
	s.published++
	s.lastTopic = topic
	return nil
}

func (s *stubPublisher) PublishForDrivers(ctx context.Context, topic string, event Event, drivers []string) error {
	return s.Publish(ctx, topic, event)
}

func (s *stubPublisher) Close() error {
	return nil
}

type stubRelayPublisher struct {
	published int
	lastTopic string
	lastMsg   relaymessage.Message
}

func (s *stubRelayPublisher) Publish(ctx context.Context, topic string, msg relaymessage.Message) error {
	s.published++
	s.lastTopic = topic
	s.lastMsg = msg
	return nil
}

func (s *stubRelayPublisher) PublishBatch(ctx context.Context, topic string, msgs []relaymessage.Message) error {
	for _, msg := range msgs {
		if err := s.Publish(ctx, topic, msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *stubRelayPublisher) Close() error {
	return nil
}

// TestRegisterPublisherDriver tests that a custom publisher driver can be registered and used.
func TestRegisterPublisherDriver(t *testing.T) {
	const driverName = "custom"

	orig, had := publisherFactories[driverName]
	defer func() {
		if had {
			publisherFactories[driverName] = orig
		} else {
			delete(publisherFactories, driverName)
		}
	}()

	stub := &stubPublisher{}
	RegisterPublisherDriver(driverName, func(cfg RelaybusConfig) (Publisher, error) {
		return stub, nil
	})

	pub, err := NewPublisher(RelaybusConfig{Driver: driverName})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	if err := pub.PublishForDrivers(context.Background(), "custom.topic", Event{Provider: "github"}, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if stub.published != 1 || stub.lastTopic != "custom.topic" {
		t.Fatalf("expected publish to custom.topic once, got %d to %q", stub.published, stub.lastTopic)
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

// TestMultipleDrivers tests that the publisher can be configured to publish to multiple drivers.
func TestMultipleDrivers(t *testing.T) {
	orig := publisherFactories["multi-a"]
	origB := publisherFactories["multi-b"]
	defer func() {
		if orig != nil {
			publisherFactories["multi-a"] = orig
		} else {
			delete(publisherFactories, "multi-a")
		}
		if origB != nil {
			publisherFactories["multi-b"] = origB
		} else {
			delete(publisherFactories, "multi-b")
		}
	}()

	a := &stubPublisher{}
	b := &stubPublisher{}

	RegisterPublisherDriver("multi-a", func(cfg RelaybusConfig) (Publisher, error) {
		return a, nil
	})
	RegisterPublisherDriver("multi-b", func(cfg RelaybusConfig) (Publisher, error) {
		return b, nil
	})

	pub, err := NewPublisher(RelaybusConfig{Drivers: []string{"multi-a", "multi-b"}})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	if err := pub.PublishForDrivers(context.Background(), "multi.topic", Event{Provider: "github"}, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if a.published != 1 || b.published != 1 {
		t.Fatalf("expected publish to both drivers, got a=%d b=%d", a.published, b.published)
	}
}

// TestPublishUsesRawPayloadAndMetadata ensures raw payload is forwarded and metadata is set.
func TestPublishUsesRawPayloadAndMetadata(t *testing.T) {
	const driverName = "payload"

	orig, had := publisherFactories[driverName]
	defer func() {
		if had {
			publisherFactories[driverName] = orig
		} else {
			delete(publisherFactories, driverName)
		}
	}()

	stub := &stubRelayPublisher{}
	RegisterPublisherDriver(driverName, func(cfg RelaybusConfig) (Publisher, error) {
		return &relaybusPublisher{publisher: stub}, nil
	})

	pub, err := NewPublisher(RelaybusConfig{Driver: driverName})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	raw := []byte(`{"hello":"world"}`)
	event := Event{
		Provider:   "github",
		Name:       "push",
		RequestID:  "req-123",
		RawPayload: raw,
	}
	if err := pub.PublishForDrivers(context.Background(), "payload.topic", event, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}

	var env cloudv1.EventPayload
	if err := proto.Unmarshal(stub.lastMsg.Payload, &env); err != nil {
		t.Fatalf("unmarshal proto payload: %v", err)
	}
	if string(env.GetPayload()) != string(raw) {
		t.Fatalf("expected raw payload to be forwarded")
	}
	if stub.lastMsg.Metadata["provider"] != "github" {
		t.Fatalf("expected provider metadata")
	}
	if stub.lastMsg.Metadata["event"] != "push" {
		t.Fatalf("expected event metadata")
	}
	if stub.lastMsg.Metadata["request_id"] != "req-123" {
		t.Fatalf("expected request_id metadata")
	}
}

func TestHTTPDriverPublishesWebhookTokenHeader(t *testing.T) {
	var headerValue string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerValue = r.Header.Get("X-Webhook-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub, err := NewPublisher(RelaybusConfig{
		Driver: "http",
		HTTP: HTTPConfig{
			Endpoint:     srv.URL + "/{topic}",
			WebhookToken: "secret-123",
		},
	})
	if err != nil {
		t.Fatalf("new http publisher: %v", err)
	}
	defer func() { _ = pub.Close() }()

	err = pub.PublishForDrivers(context.Background(), "relaybus.demo", Event{Provider: "github", Name: "push", RawPayload: []byte(`{"ok":true}`)}, nil)
	if err != nil {
		t.Fatalf("publish via http driver: %v", err)
	}
	if headerValue != "secret-123" {
		t.Fatalf("expected X-Webhook-Token header, got %q", headerValue)
	}
}
