package eventlogs

import (
	"context"
	"errors"
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

	bucketExpr, err := timeseriesBucketExpr(s.db.Dialector.Name(), interval)
	if err != nil {
		return nil, err
	}

	query := applyFilter(s.tableDB().WithContext(ctx), filter, ctx)
	type aggregateRow struct {
		BucketUnix   int64 `gorm:"column:bucket_unix"`
		EventCount   int64 `gorm:"column:event_count"`
		MatchedCount int64 `gorm:"column:matched_count"`
		DistinctReq  int64 `gorm:"column:distinct_req"`
		FailureCount int64 `gorm:"column:failure_count"`
	}
	var rows []aggregateRow
	if err := query.Select(
		bucketExpr+" as bucket_unix",
		"count(*) as event_count",
		"sum(case when matched = true then 1 else 0 end) as matched_count",
		"count(distinct case when request_id <> '' then request_id end) as distinct_req",
		"sum(case when lower(status) = 'failed' then 1 else 0 end) as failure_count",
	).Group(bucketExpr).Order("bucket_unix asc").Find(&rows).Error; err != nil {
		return nil, err
	}

	start := bucketStart(filter.StartTime.UTC(), interval)
	end := filter.EndTime.UTC()
	step := intervalDuration(interval)
	if step <= 0 {
		return nil, errors.New("invalid interval")
	}

	buckets := make(map[int64]storage.EventLogTimeseriesBucket, len(rows))
	for _, row := range rows {
		if row.BucketUnix <= 0 {
			continue
		}
		bucket := time.Unix(row.BucketUnix, 0).UTC()
		if bucket.Before(start) || bucket.After(end) {
			continue
		}
		buckets[row.BucketUnix] = storage.EventLogTimeseriesBucket{
			Start:        bucket,
			End:          bucket.Add(step),
			EventCount:   row.EventCount,
			MatchedCount: row.MatchedCount,
			DistinctReq:  row.DistinctReq,
			FailureCount: row.FailureCount,
		}
	}

	out := make([]storage.EventLogTimeseriesBucket, 0)
	for cursor := start; cursor.Before(end) || cursor.Equal(end); cursor = cursor.Add(step) {
		key := cursor.Unix()
		if entry, ok := buckets[key]; ok {
			out = append(out, entry)
		} else {
			out = append(out, storage.EventLogTimeseriesBucket{
				Start: cursor,
				End:   cursor.Add(step),
			})
		}
	}
	return out, nil
}

func timeseriesBucketExpr(dialect string, interval storage.EventLogInterval) (string, error) {
	switch interval {
	case storage.EventLogIntervalHour:
		switch dialect {
		case "postgres":
			return "CAST(EXTRACT(EPOCH FROM date_trunc('hour', created_at)) AS BIGINT)", nil
		case "mysql":
			return "UNIX_TIMESTAMP(DATE_FORMAT(created_at, '%Y-%m-%d %H:00:00'))", nil
		case "sqlite":
			return "CAST(strftime('%s', strftime('%Y-%m-%d %H:00:00', created_at)) AS INTEGER)", nil
		}
	case storage.EventLogIntervalDay:
		switch dialect {
		case "postgres":
			return "CAST(EXTRACT(EPOCH FROM date_trunc('day', created_at)) AS BIGINT)", nil
		case "mysql":
			return "UNIX_TIMESTAMP(DATE(created_at))", nil
		case "sqlite":
			return "CAST(strftime('%s', date(created_at)) AS INTEGER)", nil
		}
	case storage.EventLogIntervalWeek:
		switch dialect {
		case "postgres":
			return "CAST(EXTRACT(EPOCH FROM date_trunc('week', created_at)) AS BIGINT)", nil
		case "mysql":
			return "UNIX_TIMESTAMP(DATE_SUB(DATE(created_at), INTERVAL WEEKDAY(created_at) DAY))", nil
		case "sqlite":
			return "CAST(strftime('%s', date(created_at, '-' || ((CAST(strftime('%w', created_at) AS INTEGER) + 6) % 7) || ' days')) AS INTEGER)", nil
		}
	}
	return "", errors.New("unsupported interval")
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
