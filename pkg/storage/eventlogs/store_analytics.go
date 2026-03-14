package eventlogs

import (
	"context"
	"errors"

	"github.com/relaymesh/relaymesh/pkg/storage"

	"gorm.io/gorm"
)

// GetEventLogAnalytics returns aggregate analytics for event logs.
func (s *Store) GetEventLogAnalytics(ctx context.Context, filter storage.EventLogFilter) (storage.EventLogAnalytics, error) {
	if s == nil || s.db == nil {
		return storage.EventLogAnalytics{}, errors.New("store is not initialized")
	}
	base := applyFilter(s.tableDB().WithContext(ctx), filter, ctx)

	var total int64
	if err := base.Model(&row{}).Count(&total).Error; err != nil {
		return storage.EventLogAnalytics{}, err
	}

	var matched int64
	if err := base.Model(&row{}).Where("matched = ?", true).Count(&matched).Error; err != nil {
		return storage.EventLogAnalytics{}, err
	}

	var distinctReq int64
	if err := base.Model(&row{}).Where("request_id <> ''").Distinct("request_id").Count(&distinctReq).Error; err != nil {
		return storage.EventLogAnalytics{}, err
	}

	byProvider, err := aggregateCounts(base, "provider", "provider")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}
	byEvent, err := aggregateCounts(base, "name", "name")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}
	byTopic, err := aggregateCounts(base.Where("topic <> ''"), "topic", "topic")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}
	byRule, err := aggregateCounts(base, "COALESCE(NULLIF(rule_id,''), rule_when)", "COALESCE(NULLIF(rule_id,''), rule_when)")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}
	byInstall, err := aggregateCounts(base.Where("installation_id <> ''"), "installation_id", "installation_id")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}
	byNamespace, err := aggregateCounts(base, "COALESCE(NULLIF(namespace_name,''), namespace_id)", "COALESCE(NULLIF(namespace_name,''), namespace_id)")
	if err != nil {
		return storage.EventLogAnalytics{}, err
	}

	return storage.EventLogAnalytics{
		Total:       total,
		Matched:     matched,
		DistinctReq: distinctReq,
		ByProvider:  byProvider,
		ByEvent:     byEvent,
		ByTopic:     byTopic,
		ByRule:      byRule,
		ByInstall:   byInstall,
		ByNamespace: byNamespace,
	}, nil
}

type countRow struct {
	Key   string `gorm:"column:key"`
	Count int64  `gorm:"column:count"`
}

func aggregateCounts(query *gorm.DB, selectExpr, groupExpr string) ([]storage.EventLogCount, error) {
	var rows []countRow
	if err := query.Select(selectExpr + " as key, count(*) as count").Group(groupExpr).Order("count desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]storage.EventLogCount, 0, len(rows))
	for _, row := range rows {
		if row.Key == "" {
			continue
		}
		out = append(out, storage.EventLogCount{Key: row.Key, Count: row.Count})
	}
	return out, nil
}
