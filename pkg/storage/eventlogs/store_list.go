package eventlogs

import (
	"context"
	"errors"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/storage"
	"gorm.io/gorm"
)

// ListEventLogs returns event logs matching the filter.
func (s *Store) ListEventLogs(ctx context.Context, filter storage.EventLogFilter) ([]storage.EventLogRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	query := applyFilter(s.tableDB().WithContext(ctx), filter, ctx).
		Order("created_at desc").
		Order("id desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var data []row
	if err := query.Find(&data).Error; err != nil {
		return nil, err
	}
	out := make([]storage.EventLogRecord, 0, len(data))
	for _, item := range data {
		out = append(out, fromRow(item))
	}
	return out, nil
}

func (s *Store) GetEventLog(ctx context.Context, id string) (*storage.EventLogRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("id is required")
	}
	query := s.tableDB().WithContext(ctx).Where("id = ?", id)
	if tenantID := storage.TenantFromContext(ctx); tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	var data row
	if err := query.First(&data).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	record := fromRow(data)
	return &record, nil
}
