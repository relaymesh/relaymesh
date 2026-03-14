package eventlogs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

// GetEventLogTimeseries returns time-series buckets grouped by interval.
func (s *Store) GetEventLogTimeseries(ctx context.Context, filter storage.EventLogFilter, interval storage.EventLogInterval) ([]storage.EventLogTimeseriesBucket, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is not initialized")
	}
	if interval == "" {
		return nil, errors.New("interval is required")
	}
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, errors.New("start_time and end_time are required")
	}
	if filter.EndTime.Before(filter.StartTime) {
		return nil, errors.New("end_time must be after start_time")
	}

	query := applyFilter(s.tableDB().WithContext(ctx), filter, ctx)
	var rows []struct {
		CreatedAt time.Time `gorm:"column:created_at"`
		Matched   bool      `gorm:"column:matched"`
		RequestID string    `gorm:"column:request_id"`
		Status    string    `gorm:"column:status"`
	}
	if err := query.Select("created_at", "matched", "request_id", "status").Order("created_at asc").Find(&rows).Error; err != nil {
		return nil, err
	}

	start := bucketStart(filter.StartTime.UTC(), interval)
	end := filter.EndTime.UTC()
	step := intervalDuration(interval)
	if step <= 0 {
		return nil, errors.New("invalid interval")
	}

	type bucketData struct {
		storage.EventLogTimeseriesBucket
		reqs map[string]struct{}
	}
	buckets := make(map[int64]*bucketData)

	for _, row := range rows {
		ts := row.CreatedAt.UTC()
		if ts.Before(start) || ts.After(end) {
			continue
		}
		bucket := bucketStart(ts, interval)
		key := bucket.Unix()
		entry := buckets[key]
		if entry == nil {
			entry = &bucketData{
				EventLogTimeseriesBucket: storage.EventLogTimeseriesBucket{
					Start: bucket,
					End:   bucket.Add(step),
				},
				reqs: make(map[string]struct{}),
			}
			buckets[key] = entry
		}
		entry.EventCount++
		if row.Matched {
			entry.MatchedCount++
		}
		if row.RequestID != "" {
			entry.reqs[row.RequestID] = struct{}{}
		}
		if strings.EqualFold(row.Status, "failed") {
			entry.FailureCount++
		}
	}

	out := make([]storage.EventLogTimeseriesBucket, 0)
	for cursor := start; cursor.Before(end) || cursor.Equal(end); cursor = cursor.Add(step) {
		key := cursor.Unix()
		if entry, ok := buckets[key]; ok {
			entry.DistinctReq = int64(len(entry.reqs))
			out = append(out, entry.EventLogTimeseriesBucket)
		} else {
			out = append(out, storage.EventLogTimeseriesBucket{
				Start: cursor,
				End:   cursor.Add(step),
			})
		}
	}
	return out, nil
}

func intervalDuration(interval storage.EventLogInterval) time.Duration {
	switch interval {
	case storage.EventLogIntervalHour:
		return time.Hour
	case storage.EventLogIntervalDay:
		return 24 * time.Hour
	case storage.EventLogIntervalWeek:
		return 7 * 24 * time.Hour
	default:
		return 0
	}
}

func bucketStart(ts time.Time, interval storage.EventLogInterval) time.Time {
	ts = ts.UTC()
	switch interval {
	case storage.EventLogIntervalHour:
		return ts.Truncate(time.Hour)
	case storage.EventLogIntervalDay:
		return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC)
	case storage.EventLogIntervalWeek:
		day := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC)
		weekday := int(day.Weekday())
		shift := (weekday + 6) % 7
		return day.AddDate(0, 0, -shift)
	default:
		return ts
	}
}
