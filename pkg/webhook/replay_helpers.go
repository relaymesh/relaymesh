package webhook

import (
	"context"
	"errors"
	"log"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func BuildReplayEvent(record storage.EventLogRecord) (core.Event, error) {
	body := append([]byte(nil), record.Body...)
	if len(body) == 0 {
		if len(record.TransformedBody) > 0 {
			body = append([]byte(nil), record.TransformedBody...)
		} else {
			return core.Event{}, errors.New("event log payload is empty")
		}
	}
	rawObject, data := rawObjectAndFlatten(body)
	if rawObject == nil {
		return core.Event{}, errors.New("event log payload is invalid json")
	}
	rawObject, normalized := annotatePayload(rawObject, data, record.Provider, record.Name)
	return core.Event{
		Provider:       record.Provider,
		ProviderType:   normalized.ProviderType,
		Name:           record.Name,
		EventType:      normalized.EventType,
		Action:         normalized.Action,
		ResourceType:   normalized.ResourceType,
		ResourceID:     normalized.ResourceID,
		ResourceName:   normalized.ResourceName,
		ActorID:        normalized.ActorID,
		ActorName:      normalized.ActorName,
		OccurredAt:     normalized.OccurredAt,
		RequestID:      record.RequestID,
		Headers:        cloneHeadersMap(record.Headers),
		Data:           data,
		RawPayload:     body,
		RawObject:      rawObject,
		StateID:        record.StateID,
		TenantID:       record.TenantID,
		InstallationID: record.InstallationID,
		NamespaceID:    record.NamespaceID,
		NamespaceName:  record.NamespaceName,
		LogID:          record.ID,
	}, nil
}

func MatchRulesForEvent(ctx context.Context, event core.Event, tenantID string, ruleStore storage.RuleStore, driverStore storage.DriverStore, strict bool, logger *log.Logger) []core.MatchedRule {
	return matchRulesFromStore(ctx, event, tenantID, ruleStore, driverStore, strict, logger)
}

func RuleMatchesFromMatchedRules(rules []core.MatchedRule) []core.RuleMatch {
	return ruleMatchesFromRules(rules)
}

func cloneHeadersMap(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for k, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[k] = append([]string(nil), values...)
	}
	return out
}
