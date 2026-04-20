package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

type slackCapturePublisher struct {
	events []core.Event
	topics []string
}

func (p *slackCapturePublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	_ = ctx
	p.topics = append(p.topics, topic)
	p.events = append(p.events, event)
	return nil
}

func (p *slackCapturePublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	_ = drivers
	return p.Publish(ctx, topic, event)
}

func (p *slackCapturePublisher) Close() error { return nil }

func TestSlackHandlerURLVerification(t *testing.T) {
	body := `{"type":"url_verification","challenge":"abc123","team_id":"T1"}`
	h, err := NewSlackHandler("slack-secret", nil, &slackCapturePublisher{}, nil, 1<<20, false, nil, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("new slack handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Unix()
	ts := strconv.FormatInt(timestamp, 10)
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", computeSlackSignature("slack-secret", ts, []byte(body)))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "abc123" {
		t.Fatalf("expected challenge response, got %q", w.Body.String())
	}
}

func TestSlackHandlerPublishesMatches(t *testing.T) {
	rules := []core.Rule{{When: `provider == "slack" && action == "opened"`, Emit: core.EmitList{"slack.event"}, DriverID: "driver-1"}}
	engine, err := core.NewRuleEngine(core.RulesConfig{Rules: rules, Strict: false})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	pub := &slackCapturePublisher{}
	h, err := NewSlackHandler("slack-secret", engine, pub, nil, 1<<20, false, nil, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("new slack handler: %v", err)
	}

	body := `{"type":"event_callback","team_id":"T1","event":{"type":"message","channel":"C1","user":"U1"},"action":"opened"}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Unix()
	ts := strconv.FormatInt(timestamp, 10)
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", computeSlackSignature("slack-secret", ts, []byte(body)))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.events))
	}
	if pub.events[0].Provider != auth.ProviderSlack {
		t.Fatalf("unexpected provider: %s", pub.events[0].Provider)
	}
	if pub.events[0].ProviderType == "" {
		t.Fatalf("expected normalized provider type")
	}
}

func TestPickBestSlackInstallation(t *testing.T) {
	now := time.Now().UTC()
	records := []storage.InstallRecord{
		{TenantID: "tenant-a", InstallationID: "T1", UpdatedAt: now.Add(-2 * time.Minute), MetadataJSON: `{"app_id":"A1"}`},
		{TenantID: "tenant-b", InstallationID: "T1", UpdatedAt: now.Add(-1 * time.Minute), MetadataJSON: `{"app_id":"A2"}`},
		{TenantID: "tenant-a", InstallationID: "T1", UpdatedAt: now, MetadataJSON: `{"app_id":"A2"}`},
	}

	best := pickBestSlackInstallation(records, "tenant-a", "A2")
	if best.TenantID != "tenant-a" || best.UpdatedAt != now {
		t.Fatalf("unexpected best installation: %+v", best)
	}
}
