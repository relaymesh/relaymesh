package eventlogs

import (
	"context"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestEventLogsValidationAndHelpers(t *testing.T) {
	store, err := Open(Config{Driver: "sqlite", DSN: ":memory:", AutoMigrate: true})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := storage.WithTenant(context.Background(), "tenant-a")

	if err := store.CreateEventLogs(ctx, nil); err != nil {
		t.Fatalf("empty create should be nil: %v", err)
	}

	now := time.Now().UTC().Add(-2 * time.Minute)
	if err := store.CreateEventLogs(ctx, []storage.EventLogRecord{{
		ID:        "id-branch-1",
		Provider:  "github",
		Name:      "push",
		RequestID: "req-branch-1",
		CreatedAt: now,
		Status:    "queued",
		LatencyMS: -10,
	}}); err != nil {
		t.Fatalf("create log: %v", err)
	}

	if err := store.UpdateEventLogStatus(ctx, "", "success", ""); err == nil {
		t.Fatalf("expected id required error")
	}
	if err := store.UpdateEventLogStatus(ctx, "id-branch-1", "", ""); err == nil {
		t.Fatalf("expected status required error")
	}

	if err := store.UpdateEventLogStatus(ctx, "id-branch-1", "success", ""); err != nil {
		t.Fatalf("update success: %v", err)
	}
	if err := store.UpdateEventLogStatus(ctx, "id-branch-1", "queued", ""); err != nil {
		t.Fatalf("queued after success should be ignored, got: %v", err)
	}
	if err := store.UpdateEventLogStatus(ctx, "missing", "queued", ""); err != nil {
		t.Fatalf("queued missing should be nil, got: %v", err)
	}
	if err := store.UpdateEventLogStatus(ctx, "missing", "failed", "boom"); err == nil {
		t.Fatalf("expected missing non-queued update to fail")
	}

	if _, err := store.GetEventLogTimeseries(ctx, storage.EventLogFilter{}, storage.EventLogIntervalHour); err == nil {
		t.Fatalf("expected start/end validation error")
	}
	if _, err := store.GetEventLogTimeseries(ctx, storage.EventLogFilter{StartTime: now, EndTime: now.Add(-time.Minute)}, storage.EventLogIntervalHour); err == nil {
		t.Fatalf("expected end before start validation error")
	}
	if _, err := store.GetEventLogTimeseries(ctx, storage.EventLogFilter{StartTime: now, EndTime: now.Add(time.Minute)}, ""); err == nil {
		t.Fatalf("expected interval required error")
	}

	if _, _, err := store.GetEventLogBreakdown(ctx, storage.EventLogFilter{}, storage.EventLogBreakdownGroup("bad"), storage.EventLogBreakdownSortCount, false, 10, "", false); err == nil {
		t.Fatalf("expected unsupported group_by error")
	}
	if _, _, err := store.GetEventLogBreakdown(ctx, storage.EventLogFilter{}, storage.EventLogBreakdownProvider, storage.EventLogBreakdownSortCount, false, 10, "abc", false); err == nil {
		t.Fatalf("expected invalid page token error")
	}

	if _, err := parsePageToken("-1"); err == nil {
		t.Fatalf("expected invalid negative token")
	}
	if tok := formatPageToken(0); tok != "" {
		t.Fatalf("expected empty token for zero offset")
	}
	if tok := formatPageToken(7); tok != "7" {
		t.Fatalf("expected token 7, got %q", tok)
	}

	if got := intervalDuration(storage.EventLogInterval("bad")); got != 0 {
		t.Fatalf("expected invalid interval duration 0, got %v", got)
	}
	if got := percentile(nil, 0.95); got != 0 {
		t.Fatalf("expected percentile 0 for empty slice, got %v", got)
	}
	if got := percentile([]int64{1, 2, 3}, -1); got != 1 {
		t.Fatalf("expected low bound percentile 1, got %v", got)
	}
	if got := percentile([]int64{1, 2, 3}, 2); got != 3 {
		t.Fatalf("expected high bound percentile 3, got %v", got)
	}

	day := bucketStart(time.Date(2026, 2, 28, 11, 45, 0, 0, time.UTC), storage.EventLogIntervalDay)
	if day.Hour() != 0 || day.Minute() != 0 {
		t.Fatalf("expected day bucket at midnight, got %v", day)
	}

	week := bucketStart(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), storage.EventLogIntervalWeek)
	if week.Weekday() != time.Monday {
		t.Fatalf("expected week bucket monday, got %v", week.Weekday())
	}

	if expr, err := breakdownGroupExpr(storage.EventLogBreakdownProvider); err != nil || expr != "provider" {
		t.Fatalf("expected provider group expr, got %q err=%v", expr, err)
	}
	if expr := breakdownSortExpr(storage.EventLogBreakdownSortMatched, true); expr != "matched_count desc" {
		t.Fatalf("expected matched desc sort expr, got %q", expr)
	}
}
