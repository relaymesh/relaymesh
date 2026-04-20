package webhook

import (
	"fmt"
	"strings"
	"time"

	providerspkg "github.com/relaymesh/relaymesh/pkg/providers"
)

type normalizedEventFields struct {
	ProviderType string
	EventType    string
	Action       string
	ResourceType string
	ResourceID   string
	ResourceName string
	ActorID      string
	ActorName    string
	OccurredAt   string
}

func deriveNormalizedEventFields(provider, eventName string, data map[string]interface{}) normalizedEventFields {
	provider = strings.TrimSpace(provider)
	eventName = strings.TrimSpace(eventName)
	fields := normalizedEventFields{
		ProviderType: string(providerspkg.ProviderTypeFor(provider)),
		EventType:    eventName,
		ResourceType: normalizeResourceType(eventName),
		Action:       dataString(data, "action", "object_attributes.action", "event.action"),
		ResourceID: dataString(data,
			"repository.id",
			"pull_request.id",
			"issue.id",
			"merge_request.id",
			"object_attributes.id",
			"project.id",
			"resource.id",
		),
		ResourceName: dataString(data,
			"repository.full_name",
			"repository.name",
			"project.path_with_namespace",
			"project.path",
			"resource.name",
		),
		ActorID: dataString(data,
			"sender.id",
			"user.id",
			"actor.id",
		),
		ActorName: dataString(data,
			"sender.login",
			"sender.username",
			"user.username",
			"user.name",
			"actor.username",
			"actor.display_name",
		),
		OccurredAt: normalizeTimestamp(
			dataString(data,
				"head_commit.timestamp",
				"repository.updated_at",
				"pull_request.updated_at",
				"issue.updated_at",
				"object_attributes.updated_at",
				"object_attributes.created_at",
				"event.timestamp",
			),
		),
	}
	return fields
}

func normalizeResourceType(eventName string) string {
	eventName = strings.TrimSpace(strings.ToLower(eventName))
	if eventName == "" {
		return ""
	}
	eventName = strings.ReplaceAll(eventName, ".", "_")
	eventName = strings.ReplaceAll(eventName, ":", "_")
	return eventName
}

func dataString(data map[string]interface{}, keys ...string) string {
	if data == nil {
		return ""
	}
	for _, key := range keys {
		value, ok := data[key]
		if !ok || value == nil {
			continue
		}
		trimmed := strings.TrimSpace(fmt.Sprintf("%v", value))
		if trimmed != "" && trimmed != "<nil>" {
			return trimmed
		}
	}
	return ""
}

func normalizeTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return value
}
