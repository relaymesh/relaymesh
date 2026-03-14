package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func logDebugEvent(logger *log.Logger, provider string, event string, body []byte) {
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("debug event provider=%s name=%s payload=%s", provider, event, string(body))
}

func logEventFailure(ctx context.Context, store storage.EventLogStore, logger *log.Logger, event core.Event, reason string) {
	if store == nil {
		return
	}
	headers := cloneHeaders(event.Headers)
	body := append([]byte(nil), event.RawPayload...)
	bodyHash := hashBody(body)
	record := storage.EventLogRecord{
		ID:             uuid.NewString(),
		Provider:       event.Provider,
		Name:           event.Name,
		RequestID:      event.RequestID,
		StateID:        event.StateID,
		InstallationID: event.InstallationID,
		NamespaceID:    event.NamespaceID,
		NamespaceName:  event.NamespaceName,
		Headers:        headers,
		Body:           body,
		BodyHash:       bodyHash,
		Status:         eventLogStatusFailed,
		ErrorMessage:   reason,
		Matched:        false,
	}
	if err := store.CreateEventLogs(ctx, []storage.EventLogRecord{record}); err != nil && logger != nil {
		logger.Printf("event log write failed: %v", err)
	}
}

func logEventMatches(ctx context.Context, store storage.EventLogStore, logger *log.Logger, event core.Event, rules []core.MatchedRule) []storage.EventLogRecord {
	if store == nil {
		return nil
	}
	records, matched := buildEventLogRecords(event, rules)
	if len(records) == 0 {
		return nil
	}
	if err := store.CreateEventLogs(ctx, records); err != nil && logger != nil {
		logger.Printf("event log write failed: %v", err)
	}
	return matched
}

func buildEventLogRecords(event core.Event, rules []core.MatchedRule) ([]storage.EventLogRecord, []storage.EventLogRecord) {
	headers := cloneHeaders(event.Headers)
	body := append([]byte(nil), event.RawPayload...)
	bodyHash := hashBody(body)
	if len(rules) == 0 {
		record := storage.EventLogRecord{
			ID:             uuid.NewString(),
			Provider:       event.Provider,
			Name:           event.Name,
			RequestID:      event.RequestID,
			StateID:        event.StateID,
			InstallationID: event.InstallationID,
			NamespaceID:    event.NamespaceID,
			NamespaceName:  event.NamespaceName,
			Headers:        headers,
			Body:           body,
			BodyHash:       bodyHash,
			Status:         eventLogStatusIgnored,
			Matched:        false,
		}
		return []storage.EventLogRecord{record}, nil
	}

	records := make([]storage.EventLogRecord, 0, len(rules))
	matched := make([]storage.EventLogRecord, 0, len(rules))
	for _, rule := range rules {
		driverName := strings.TrimSpace(rule.DriverName)
		if driverName == "" {
			driverName = strings.TrimSpace(rule.DriverID)
		}
		var drivers []string
		if driverName != "" {
			drivers = []string{driverName}
		}
		for _, topic := range rule.Emit {
			record := storage.EventLogRecord{
				ID:             uuid.NewString(),
				Provider:       event.Provider,
				Name:           event.Name,
				RequestID:      event.RequestID,
				StateID:        event.StateID,
				InstallationID: event.InstallationID,
				NamespaceID:    event.NamespaceID,
				NamespaceName:  event.NamespaceName,
				Topic:          topic,
				RuleID:         rule.ID,
				RuleWhen:       rule.When,
				Drivers:        append([]string(nil), drivers...),
				Headers:        headers,
				Body:           body,
				BodyHash:       bodyHash,
				Status:         eventLogStatusQueued,
				Matched:        true,
			}
			records = append(records, record)
			matched = append(matched, record)
		}
	}
	return records, matched
}

func topicsFromLogRecords(records []storage.EventLogRecord) []string {
	topics := make([]string, 0, len(records))
	for _, record := range records {
		if record.Topic == "" {
			continue
		}
		topics = append(topics, record.Topic)
	}
	return topics
}

func hashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
