package webhook

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestLogEventMatchesAndFailure(t *testing.T) {
	store := storage.NewMockEventLogStore()
	event := core.Event{
		Provider:       "github",
		Name:           "push",
		RequestID:      "req-1",
		InstallationID: "inst",
	}
	rules := []core.MatchedRule{
		{ID: "rule", When: "action == \"opened\"", Emit: []string{"topic"}, DriverID: "amqp"},
	}
	records := logEventMatches(context.Background(), store, nil, event, rules)
	if len(records) != 1 {
		t.Fatalf("expected 1 log record")
	}

	logEventFailure(context.Background(), store, nil, event, "failed")
	list, err := store.ListEventLogs(context.Background(), storage.EventLogFilter{RequestID: "req-1"})
	if err != nil {
		t.Fatalf("list event logs: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected failure log")
	}
}
