package eventlogs

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"gorm.io/gorm"
)

// CreateEventLogs inserts event log records.
func (s *Store) CreateEventLogs(ctx context.Context, records []storage.EventLogRecord) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	if len(records) == 0 {
		return nil
	}
	now := time.Now().UTC()
	rows := make([]row, 0, len(records))
	for _, record := range records {
		if record.TenantID == "" {
			record.TenantID = storage.TenantFromContext(ctx)
		}
		if record.ID == "" {
			record.ID = uuid.NewString()
		}
		if record.Status == "" {
			record.Status = "queued"
		}
		if record.CreatedAt.IsZero() {
			record.CreatedAt = now
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = record.CreatedAt
		}
		if record.LatencyMS < 0 {
			record.LatencyMS = 0
		}
		data, err := toRow(record)
		if err != nil {
			return err
		}
		rows = append(rows, data)
	}
	return s.tableDB().WithContext(ctx).Create(&rows).Error
}

// UpdateEventLogStatus updates the status and error message of an event log.
func (s *Store) UpdateEventLogStatus(ctx context.Context, id, status, errorMessage string) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" {
		return errors.New("id is required")
	}
	if status == "" {
		return errors.New("status is required")
	}
	tenantID := storage.TenantFromContext(ctx)
	buildQuery := func() *gorm.DB {
		q := s.tableDB().WithContext(ctx).Where("id = ?", id)
		if tenantID != "" {
			q = q.Where("tenant_id = ?", tenantID)
		}
		return q
	}
	updates := map[string]interface{}{
		"status":        status,
		"error_message": strings.TrimSpace(errorMessage),
		"updated_at":    time.Now().UTC(),
	}
	if status == "success" || status == "failed" {
		var existing row
		lookup := buildQuery()
		if err := lookup.Select("created_at").First(&existing).Error; err == nil && !existing.CreatedAt.IsZero() {
			updates["latency_ms"] = time.Since(existing.CreatedAt).Milliseconds()
		}
	}
	updateQuery := buildQuery()
	if status == "queued" {
		updateQuery = updateQuery.Where("status <> ? AND status <> ?", "success", "failed")
	}
	result := updateQuery.Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if status == "queued" {
			return nil
		}
		return errors.New("event log not found")
	}
	log.Printf("event log update rows=%d id=%s status=%s tenant=%s", result.RowsAffected, id, status, tenantID)
	var current row
	verifyQuery := buildQuery()
	if err := verifyQuery.Select("status", "updated_at").First(&current).Error; err == nil {
		log.Printf("event log update verify id=%s status=%s tenant=%s updated_at=%s", id, current.Status, tenantID, current.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	return nil
}

func (s *Store) UpdateEventLogTransformedPayload(ctx context.Context, id string, transformedBody []byte) error {
	if s == nil || s.db == nil {
		return errors.New("store is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	tenantID := storage.TenantFromContext(ctx)
	query := s.tableDB().WithContext(ctx).Where("id = ?", id)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	result := query.Updates(map[string]interface{}{
		"transformed_body": string(transformedBody),
		"updated_at":       time.Now().UTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("event log not found")
	}
	return nil
}
