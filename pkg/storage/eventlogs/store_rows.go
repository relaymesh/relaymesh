package eventlogs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func toRow(record storage.EventLogRecord) (row, error) {
	driversJSON, err := json.Marshal(record.Drivers)
	if err != nil {
		return row{}, err
	}
	headersJSON := ""
	if len(record.Headers) > 0 {
		raw, err := json.Marshal(record.Headers)
		if err != nil {
			return row{}, err
		}
		headersJSON = string(raw)
	}
	bodyHash := record.BodyHash
	if bodyHash == "" {
		bodyHash = hashBody(record.Body)
	}
	return row{
		ID:              record.ID,
		TenantID:        record.TenantID,
		Provider:        record.Provider,
		Name:            record.Name,
		RequestID:       record.RequestID,
		StateID:         record.StateID,
		InstallationID:  record.InstallationID,
		NamespaceID:     record.NamespaceID,
		NamespaceName:   record.NamespaceName,
		Topic:           record.Topic,
		RuleID:          record.RuleID,
		RuleWhen:        record.RuleWhen,
		DriversJSON:     string(driversJSON),
		HeadersJSON:     headersJSON,
		Body:            string(record.Body),
		TransformedBody: string(record.TransformedBody),
		BodyHash:        bodyHash,
		Matched:         record.Matched,
		Status:          record.Status,
		ErrorMessage:    record.ErrorMessage,
		LatencyMS:       record.LatencyMS,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
	}, nil
}

func fromRow(data row) storage.EventLogRecord {
	record := storage.EventLogRecord{
		ID:              data.ID,
		TenantID:        data.TenantID,
		Provider:        data.Provider,
		Name:            data.Name,
		RequestID:       data.RequestID,
		StateID:         data.StateID,
		InstallationID:  data.InstallationID,
		NamespaceID:     data.NamespaceID,
		NamespaceName:   data.NamespaceName,
		Topic:           data.Topic,
		RuleID:          data.RuleID,
		RuleWhen:        data.RuleWhen,
		Headers:         nil,
		Body:            []byte(data.Body),
		TransformedBody: []byte(data.TransformedBody),
		BodyHash:        data.BodyHash,
		Matched:         data.Matched,
		Status:          data.Status,
		ErrorMessage:    data.ErrorMessage,
		LatencyMS:       data.LatencyMS,
		CreatedAt:       data.CreatedAt,
		UpdatedAt:       data.UpdatedAt,
	}
	if data.DriversJSON != "" {
		_ = json.Unmarshal([]byte(data.DriversJSON), &record.Drivers)
	}
	if data.HeadersJSON != "" {
		_ = json.Unmarshal([]byte(data.HeadersJSON), &record.Headers)
	}
	return record
}

func hashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
