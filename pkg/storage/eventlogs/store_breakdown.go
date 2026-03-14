package eventlogs

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

// GetEventLogBreakdown returns grouped aggregates and an optional next page token.
func (s *Store) GetEventLogBreakdown(ctx context.Context, filter storage.EventLogFilter, groupBy storage.EventLogBreakdownGroup, sortBy storage.EventLogBreakdownSort, sortDesc bool, pageSize int, pageToken string, includeLatency bool) ([]storage.EventLogBreakdown, string, error) {
	if s == nil || s.db == nil {
		return nil, "", errors.New("store is not initialized")
	}
	groupExpr, err := breakdownGroupExpr(groupBy)
	if err != nil {
		return nil, "", err
	}
	orderExpr := breakdownSortExpr(sortBy, sortDesc)
	offset, err := parsePageToken(pageToken)
	if err != nil {
		return nil, "", err
	}
	if pageSize <= 0 {
		pageSize = 50
	}

	query := applyFilter(s.tableDB().WithContext(ctx), filter, ctx)
	selectExpr := strings.Join([]string{
		groupExpr + " as key",
		"count(*) as count",
		"sum(case when matched = true then 1 else 0 end) as matched_count",
		"sum(case when status = 'failed' then 1 else 0 end) as failed_count",
	}, ", ")

	type breakdownRow struct {
		Key          string `gorm:"column:key"`
		Count        int64  `gorm:"column:count"`
		MatchedCount int64  `gorm:"column:matched_count"`
		FailedCount  int64  `gorm:"column:failed_count"`
	}
	var rows []breakdownRow
	if err := query.Select(selectExpr).Group(groupExpr).Order(orderExpr).Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, "", err
	}

	out := make([]storage.EventLogBreakdown, 0, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Key) == "" {
			continue
		}
		keys = append(keys, row.Key)
		out = append(out, storage.EventLogBreakdown{
			Key:          row.Key,
			EventCount:   row.Count,
			MatchedCount: row.MatchedCount,
			FailureCount: row.FailedCount,
		})
	}

	if includeLatency && len(keys) > 0 {
		stats, err := s.fetchLatencyByGroup(ctx, filter, groupExpr, keys)
		if err != nil {
			return nil, "", err
		}
		for i := range out {
			if values, ok := stats[out[i].Key]; ok {
				out[i].LatencyP50MS = values.P50
				out[i].LatencyP95MS = values.P95
				out[i].LatencyP99MS = values.P99
			}
		}
	}

	nextToken := ""
	if len(rows) == pageSize {
		nextToken = formatPageToken(offset + pageSize)
	}
	return out, nextToken, nil
}

func breakdownGroupExpr(groupBy storage.EventLogBreakdownGroup) (string, error) {
	switch groupBy {
	case storage.EventLogBreakdownProvider:
		return "provider", nil
	case storage.EventLogBreakdownEvent:
		return "name", nil
	case storage.EventLogBreakdownRuleID:
		return "rule_id", nil
	case storage.EventLogBreakdownRuleWhen:
		return "rule_when", nil
	case storage.EventLogBreakdownTopic:
		return "topic", nil
	case storage.EventLogBreakdownNamespaceID:
		return "namespace_id", nil
	case storage.EventLogBreakdownNamespaceName:
		return "namespace_name", nil
	case storage.EventLogBreakdownInstallation:
		return "installation_id", nil
	default:
		return "", errors.New("unsupported group_by")
	}
}

func breakdownSortExpr(sortBy storage.EventLogBreakdownSort, sortDesc bool) string {
	column := ""
	switch sortBy {
	case storage.EventLogBreakdownSortMatched:
		column = "matched_count"
	case storage.EventLogBreakdownSortFailed:
		column = "failed_count"
	case storage.EventLogBreakdownSortCount:
		column = "count"
	default:
		column = "count"
	}
	if sortDesc {
		return column + " desc"
	}
	return column + " asc"
}

func parsePageToken(token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, errors.New("invalid page_token")
	}
	return offset, nil
}

func formatPageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	return strconv.Itoa(offset)
}

type latencyStats struct {
	P50 float64
	P95 float64
	P99 float64
}

func (s *Store) fetchLatencyByGroup(ctx context.Context, filter storage.EventLogFilter, groupExpr string, keys []string) (map[string]latencyStats, error) {
	query := applyFilter(s.tableDB().WithContext(ctx), filter, ctx)
	type latencyRow struct {
		Key       string `gorm:"column:key"`
		LatencyMS int64  `gorm:"column:latency_ms"`
	}
	var rows []latencyRow
	if err := query.Select(groupExpr+" as key", "latency_ms").
		Where("latency_ms > 0").
		Where(groupExpr+" IN ?", keys).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	grouped := make(map[string][]int64)
	for _, row := range rows {
		if strings.TrimSpace(row.Key) == "" {
			continue
		}
		grouped[row.Key] = append(grouped[row.Key], row.LatencyMS)
	}

	out := make(map[string]latencyStats, len(grouped))
	for key, values := range grouped {
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		out[key] = latencyStats{
			P50: percentile(values, 0.50),
			P95: percentile(values, 0.95),
			P99: percentile(values, 0.99),
		}
	}
	return out, nil
}

func percentile(values []int64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return float64(values[0])
	}
	if p >= 1 {
		return float64(values[len(values)-1])
	}
	index := int(float64(len(values)-1) * p)
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return float64(values[index])
}
