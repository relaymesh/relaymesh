package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
)

type jiraCapturePublisher struct {
	events []core.Event
	topics []string
}

func (p *jiraCapturePublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	_ = ctx
	p.topics = append(p.topics, topic)
	p.events = append(p.events, event)
	return nil
}

func (p *jiraCapturePublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	_ = drivers
	return p.Publish(ctx, topic, event)
}

func (p *jiraCapturePublisher) Close() error { return nil }

func TestJiraHandlerPublishesMatches(t *testing.T) {
	rules := []core.Rule{{When: `provider == "atlassian" && webhookEvent == "jira:issue_created"`, Emit: core.EmitList{"atlassian.issue.created"}, DriverID: "driver-1"}}
	engine, err := core.NewRuleEngine(core.RulesConfig{Rules: rules, Strict: false})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	pub := &jiraCapturePublisher{}
	h, err := NewJiraHandler("", engine, pub, nil, 1<<20, false, nil, nil, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("new jira handler: %v", err)
	}

	body := `{"webhookEvent":"jira:issue_created","issue":{"self":"https://acme.atlassian.net/rest/api/3/issue/10000","fields":{"project":{"id":"10001","key":"ENG"}}}}`
	req := httptest.NewRequest(http.MethodPost, "/webhooks/jira", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(pub.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.events))
	}
	if pub.events[0].Provider != auth.ProviderAtlassian {
		t.Fatalf("unexpected provider: %s", pub.events[0].Provider)
	}
	if pub.events[0].NamespaceName != "ENG" {
		t.Fatalf("expected namespace ENG, got %q", pub.events[0].NamespaceName)
	}
}
