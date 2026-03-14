package worker

import (
	"encoding/json"
	"testing"

	relaymessage "github.com/relaymesh/relaybus/sdk/core/go/message"
	"google.golang.org/protobuf/proto"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
)

func TestDefaultCodecDecodeProto(t *testing.T) {
	payload := &cloudv1.EventPayload{
		Provider: "github",
		Name:     "push",
		Payload:  []byte(`{"action":"opened"}`),
	}
	raw, err := proto.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &relaymessage.Message{Payload: raw}

	event, err := (DefaultCodec{}).Decode("topic", msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if event.Provider != "github" || event.Type != "push" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Normalized["action"] != "opened" {
		t.Fatalf("unexpected data: %v", event.Normalized)
	}
}

func TestDefaultCodecDecodeJSONFallback(t *testing.T) {
	rawJSON := map[string]interface{}{
		"provider": "gitlab",
		"name":     "merge",
		"data": map[string]interface{}{
			"ref": "refs/heads/main",
		},
	}
	raw, err := json.Marshal(rawJSON)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	msg := &relaymessage.Message{Payload: raw}
	event, err := (DefaultCodec{}).Decode("topic", msg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if event.Provider != "gitlab" || event.Type != "merge" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.Normalized["ref"] != "refs/heads/main" {
		t.Fatalf("unexpected data: %v", event.Normalized)
	}
}
