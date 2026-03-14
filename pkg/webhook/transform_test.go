package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

type capturePublisher struct {
	events []core.Event
}

func (p *capturePublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	p.events = append(p.events, event)
	return nil
}

func (p *capturePublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	p.events = append(p.events, event)
	return nil
}

func (p *capturePublisher) Close() error { return nil }

func TestApplyRuleTransform(t *testing.T) {
	evt := core.Event{RawPayload: []byte(`{"action":"opened"}`)}
	out, err := applyRuleTransform(evt, `function transform(payload){ payload.changed = true; return payload; }`)
	if err != nil {
		t.Fatalf("apply transform: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(out.RawPayload, &data); err != nil {
		t.Fatalf("unmarshal transformed payload: %v", err)
	}
	if data["changed"] != true {
		t.Fatalf("expected transformed field, got %v", data)
	}

	if _, err := applyRuleTransform(evt, `bad javascript`); err == nil {
		t.Fatalf("expected compile failure")
	}
	if _, err := applyRuleTransform(evt, `({ value: 1 })`); err == nil {
		t.Fatalf("expected missing transform function")
	}

	ctxEvt := core.Event{RawPayload: []byte(`{"ref":"refs/heads/main"}`), Provider: "github", Name: "push", RequestID: "req-1"}
	ctxOut, err := applyRuleTransform(ctxEvt, `function transform(payload, event){ payload.provider = event.provider; payload.request = event.request_id; return payload; }`)
	if err != nil {
		t.Fatalf("apply transform with event context: %v", err)
	}
	var ctxPayload map[string]interface{}
	if err := json.Unmarshal(ctxOut.RawPayload, &ctxPayload); err != nil {
		t.Fatalf("unmarshal context payload: %v", err)
	}
	if ctxPayload["provider"] != "github" || ctxPayload["request"] != "req-1" {
		t.Fatalf("expected event context fields, got %v", ctxPayload)
	}

	envOut, err := applyRuleTransform(ctxEvt, `function transform(payload, event){ event.payload.ref_name = "main"; return event; }`)
	if err != nil {
		t.Fatalf("apply transform returning event envelope: %v", err)
	}
	var envPayload map[string]interface{}
	if err := json.Unmarshal(envOut.RawPayload, &envPayload); err != nil {
		t.Fatalf("unmarshal envelope payload: %v", err)
	}
	if envPayload["ref_name"] != "main" {
		t.Fatalf("expected payload extracted from envelope, got %v", envPayload)
	}
}

func TestPublishMatchesWithFallbackAppliesTransform(t *testing.T) {
	fallback := &capturePublisher{}
	event := core.Event{RawPayload: []byte(`{"value":1}`), Provider: "github", Name: "push"}
	matches := []core.RuleMatch{
		{Topic: "topic.ok", DriverName: "amqp", RuleID: "r1", TransformJS: `function transform(payload){ payload.value = payload.value + 1; return payload; }`},
		{Topic: "topic.bad", DriverName: "amqp", RuleID: "r2", TransformJS: `bad js`},
	}
	logs := []storage.EventLogRecord{{ID: "log-1"}, {ID: "log-2"}}

	updates := map[string]string{}
	payloadUpdates := map[string][]byte{}
	publishMatchesWithFallback(context.Background(), event, matches, logs, nil, fallback, nil, func(id, status, _ string) {
		updates[id] = status
	}, func(id string, transformed []byte) {
		payloadUpdates[id] = append([]byte(nil), transformed...)
	})

	if len(fallback.events) != 1 {
		t.Fatalf("expected one successful publish, got %d", len(fallback.events))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(fallback.events[0].RawPayload, &payload); err != nil {
		t.Fatalf("unmarshal published payload: %v", err)
	}
	if payload["value"] != float64(2) {
		t.Fatalf("expected transformed payload value=2, got %v", payload)
	}
	if updates["log-2"] != eventLogStatusFailed {
		t.Fatalf("expected failed status update for transform error, got %v", updates)
	}
	if len(payloadUpdates["log-1"]) == 0 {
		t.Fatalf("expected transformed payload update for successful match")
	}
}
