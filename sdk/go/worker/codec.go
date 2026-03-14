package worker

import (
	"encoding/json"
	"errors"
	"strings"

	relaymessage "github.com/relaymesh/relaybus/sdk/core/go/message"
	"google.golang.org/protobuf/proto"

	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
)

// Codec is an interface for decoding messages from a message broker into an Event.
type Codec interface {
	// Decode transforms a Relaybus message into an Event.
	Decode(topic string, msg *relaymessage.Message) (*Event, error)
}

// DefaultCodec is the default implementation of the Codec interface.
// It decodes a protobuf EventPayload into an Event, with a JSON fallback.
type DefaultCodec struct{}

// Decode unmarshals a Relaybus message into an Event.
func (DefaultCodec) Decode(topic string, msg *relaymessage.Message) (*Event, error) {
	if msg == nil {
		return nil, errors.New("message is nil")
	}
	var (
		provider   string
		eventName  string
		raw        []byte
		normalized map[string]interface{}
	)

	var env cloudv1.EventPayload
	if protoErr := proto.Unmarshal(msg.Payload, &env); protoErr == nil {
		provider = env.GetProvider()
		eventName = env.GetName()
		raw = env.GetPayload()
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &normalized)
		}
	} else {
		var legacy map[string]interface{}
		if jsonErr := json.Unmarshal(msg.Payload, &legacy); jsonErr != nil {
			return nil, protoErr
		}
		if val, ok := legacy["provider"].(string); ok {
			provider = val
		}
		if val, ok := legacy["name"].(string); ok {
			eventName = val
		}
		if data, ok := legacy["data"].(map[string]interface{}); ok {
			normalized = data
		}
		raw = msg.Payload
	}

	metadata := make(map[string]string, len(msg.Metadata))
	for key, value := range msg.Metadata {
		metadata[key] = value
	}

	if provider == "" {
		provider = metadataValue(msg.Metadata, MetadataKeyProvider)
	}
	if eventName == "" {
		eventName = metadataValue(msg.Metadata, MetadataKeyEvent)
	}

	if normalized == nil {
		var rawJSON interface{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &rawJSON); err == nil {
				if object, ok := rawJSON.(map[string]interface{}); ok {
					normalized = object
				}
			}
		}
	}

	payload := json.RawMessage(raw)
	return &Event{
		Provider:   provider,
		Type:       eventName,
		Topic:      resolveTopic(topic, msg),
		Metadata:   metadata,
		Payload:    payload,
		Normalized: normalized,
	}, nil
}

func metadataValue(meta map[string]string, key string) string {
	if meta == nil {
		return ""
	}
	return meta[key]
}

func resolveTopic(topic string, msg *relaymessage.Message) string {
	if strings.TrimSpace(topic) != "" {
		return topic
	}
	if msg == nil {
		return ""
	}
	return msg.Topic
}
