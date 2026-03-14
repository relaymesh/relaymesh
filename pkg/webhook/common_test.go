package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestRawObjectAndFlatten(t *testing.T) {
	raw := []byte(`{"action":"opened","pull_request":{"draft":false}}`)
	obj, data := rawObjectAndFlatten(raw)
	if obj == nil {
		t.Fatalf("expected object")
	}
	if data["action"] != "opened" || data["pull_request.draft"] != false {
		t.Fatalf("unexpected flattened data: %v", data)
	}

	obj, data = rawObjectAndFlatten([]byte(`invalid`))
	if obj != nil {
		t.Fatalf("expected nil object for invalid json")
	}
	if len(data) != 0 {
		t.Fatalf("expected empty data for invalid json, got %v", data)
	}
}

func TestAnnotatePayload(t *testing.T) {
	raw := map[string]interface{}{"foo": "bar"}
	data := map[string]interface{}{"x": "y"}
	out := annotatePayload(raw, data, "github", "push")
	obj, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map output")
	}
	if obj["provider"] != "github" || obj["event"] != "push" {
		t.Fatalf("unexpected annotations: %v", obj)
	}
	if data["provider"] != "github" || data["event"] != "push" {
		t.Fatalf("unexpected data annotations: %v", data)
	}
}

func TestRequestID(t *testing.T) {
	if id := requestID(nil); id == "" {
		t.Fatalf("expected request id")
	}
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set("X-Request-Id", "req-1")
	if id := requestID(req); id != "req-1" {
		t.Fatalf("expected request id from header, got %q", id)
	}
	req.Header.Del("X-Request-Id")
	req.Header.Set("X-Request-ID", "req-2")
	if id := requestID(req); id != "req-2" {
		t.Fatalf("expected request id from header, got %q", id)
	}
	req.Header.Del("X-Request-ID")
	req.Header.Set("X-Correlation-Id", "req-3")
	if id := requestID(req); id != "req-3" {
		t.Fatalf("expected request id from header, got %q", id)
	}
}

func TestRuleMatchesAndTopics(t *testing.T) {
	rules := []core.MatchedRule{
		{ID: "1", Emit: []string{"a", "b"}, DriverID: "amqp"},
	}
	matches := ruleMatchesFromRules(rules)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	topics := topicsFromMatches(matches)
	if len(topics) != 2 || topics[0] != "a" || topics[1] != "b" {
		t.Fatalf("unexpected topics: %v", topics)
	}
}

func TestDriverListFromMatchAndRefHelpers(t *testing.T) {
	if drivers := driverListFromMatch(core.RuleMatch{DriverName: "amqp"}); len(drivers) != 1 || drivers[0] != "amqp" {
		t.Fatalf("unexpected drivers from driver name: %v", drivers)
	}
	if drivers := driverListFromMatch(core.RuleMatch{DriverID: "driver-id"}); len(drivers) != 1 || drivers[0] != "driver-id" {
		t.Fatalf("unexpected drivers from driver id: %v", drivers)
	}
	if drivers := driverListFromMatch(core.RuleMatch{}); drivers != nil {
		t.Fatalf("expected nil drivers for empty match, got %v", drivers)
	}

	if ref, ok := normalizeGitRef("main"); !ok || ref != "refs/heads/main" {
		t.Fatalf("unexpected normalized ref: %q ok=%v", ref, ok)
	}
	if ref, ok := normalizeGitRef("refs/tags/v1"); !ok || ref != "refs/tags/v1" {
		t.Fatalf("unexpected normalized tag ref: %q ok=%v", ref, ok)
	}
	if ref, ok := normalizeGitRef(" "); ok || ref != "" {
		t.Fatalf("expected invalid empty ref")
	}
}

func TestCloneHeadersAndHashBody(t *testing.T) {
	if cloned := cloneHeaders(nil); cloned != nil {
		t.Fatalf("expected nil cloned headers")
	}

	headers := http.Header{"X-Test": []string{"a", "b"}}
	cloned := cloneHeaders(headers)
	cloned["X-Test"][0] = "changed"
	if headers.Get("X-Test") != "a" {
		t.Fatalf("expected original headers to remain unchanged")
	}

	if got := hashBody(nil); got != "" {
		t.Fatalf("expected empty hash for empty body")
	}
	if got := hashBody([]byte("abc")); len(got) != 64 {
		t.Fatalf("expected sha256 hash length 64, got %d", len(got))
	}
}

func TestBuildEventLogRecords(t *testing.T) {
	event := core.Event{
		Provider:       "github",
		Name:           "push",
		RequestID:      "req-1",
		InstallationID: "inst-1",
		NamespaceID:    "repo-1",
		NamespaceName:  "owner/repo",
	}
	records, matched := buildEventLogRecords(event, nil)
	if len(records) != 1 || len(matched) != 0 {
		t.Fatalf("expected unmatched record")
	}
	if records[0].Status != eventLogStatusIgnored || records[0].Matched {
		t.Fatalf("unexpected record status: %+v", records[0])
	}
	if records[0].RequestID != "req-1" {
		t.Fatalf("expected request id")
	}

	rules := []core.MatchedRule{
		{ID: "rule-1", When: "action == \"opened\"", Emit: []string{"topic-1"}, DriverID: "amqp"},
	}
	records, matched = buildEventLogRecords(event, rules)
	if len(records) != 1 || len(matched) != 1 {
		t.Fatalf("expected matched record")
	}
	if records[0].Status != eventLogStatusQueued || !records[0].Matched {
		t.Fatalf("unexpected matched record status: %+v", records[0])
	}
	if len(records[0].Drivers) != 1 || records[0].Drivers[0] != "amqp" {
		t.Fatalf("unexpected drivers: %v", records[0].Drivers)
	}
}

func TestTopicsFromLogRecords(t *testing.T) {
	records := []storage.EventLogRecord{
		{Topic: "a"},
		{Topic: ""},
		{Topic: "b"},
	}
	topics := topicsFromLogRecords(records)
	if len(topics) != 2 || topics[0] != "a" || topics[1] != "b" {
		t.Fatalf("unexpected topics: %v", topics)
	}
}

func TestRequestIDHasFallback(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	id := requestID(req)
	if id == "" {
		t.Fatalf("expected request id fallback")
	}
}

func TestPrepareWebhookRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", io.NopCloser(strings.NewReader(`{"action":"opened"}`)))
	rec := httptest.NewRecorder()

	outReq, logger, reqID, body, ok := prepareWebhookRequest(rec, req, 1024, nil)
	if !ok {
		t.Fatalf("expected successful request preparation")
	}
	if outReq == nil || logger == nil {
		t.Fatalf("expected non-nil request and logger")
	}
	if reqID == "" {
		t.Fatalf("expected request id")
	}
	if string(body) != `{"action":"opened"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Fatalf("expected response X-Request-Id header")
	}
}
