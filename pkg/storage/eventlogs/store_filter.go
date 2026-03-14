package eventlogs

import (
	"context"

	"github.com/relaymesh/relaymesh/pkg/storage"

	"gorm.io/gorm"
)

func applyFilter(query *gorm.DB, filter storage.EventLogFilter, ctx context.Context) *gorm.DB {
	query = query.Scopes(storage.TenantScope(ctx, filter.TenantID, "tenant_id"))
	if filter.Provider != "" {
		query = query.Where("provider = ?", filter.Provider)
	}
	if filter.Name != "" {
		query = query.Where("name = ?", filter.Name)
	}
	if filter.RequestID != "" {
		query = query.Where("request_id = ?", filter.RequestID)
	}
	if filter.StateID != "" {
		query = query.Where("state_id = ?", filter.StateID)
	}
	if filter.InstallationID != "" {
		query = query.Where("installation_id = ?", filter.InstallationID)
	}
	if filter.NamespaceID != "" {
		query = query.Where("namespace_id = ?", filter.NamespaceID)
	}
	if filter.NamespaceName != "" {
		query = query.Where("namespace_name = ?", filter.NamespaceName)
	}
	if filter.Topic != "" {
		query = query.Where("topic = ?", filter.Topic)
	}
	if filter.RuleID != "" {
		query = query.Where("rule_id = ?", filter.RuleID)
	}
	if filter.RuleWhen != "" {
		query = query.Where("rule_when = ?", filter.RuleWhen)
	}
	if filter.Matched != nil {
		query = query.Where("matched = ?", *filter.Matched)
	}
	if !filter.StartTime.IsZero() {
		query = query.Where("created_at >= ?", filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query = query.Where("created_at <= ?", filter.EndTime)
	}
	return query
}
