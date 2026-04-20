package api

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestInstallationsServiceLifecycle(t *testing.T) {
	store := storage.NewMockStore()
	service := &InstallationsService{Store: store}
	ctx := storage.WithTenant(context.Background(), "tenant-a")

	upsertReq := connect.NewRequest(&cloudv1.UpsertInstallationRequest{
		Installation: &cloudv1.InstallRecord{
			Provider:       "github",
			AccountId:      "acct",
			AccountName:    "account",
			InstallationId: "inst",
		},
	})
	if _, err := service.UpsertInstallation(ctx, upsertReq); err != nil {
		t.Fatalf("upsert installation: %v", err)
	}

	listResp, err := service.ListInstallations(ctx, connect.NewRequest(&cloudv1.ListInstallationsRequest{
		Provider: "github",
	}))
	if err != nil {
		t.Fatalf("list installations: %v", err)
	}
	if len(listResp.Msg.GetInstallations()) != 1 {
		t.Fatalf("expected 1 installation")
	}

	if _, err := service.GetInstallationByID(ctx, connect.NewRequest(&cloudv1.GetInstallationByIDRequest{
		Provider:       "github",
		InstallationId: "missing",
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected not found error")
	}

	if _, err := service.DeleteInstallation(ctx, connect.NewRequest(&cloudv1.DeleteInstallationRequest{
		Provider:       "github",
		AccountId:      "acct",
		InstallationId: "inst",
	})); err != nil {
		t.Fatalf("delete installation: %v", err)
	}
}

func TestDriversServiceLifecycle(t *testing.T) {
	store := storage.NewMockDriverStore()
	service := &DriversService{Store: store}
	ctx := storage.WithTenant(context.Background(), "tenant-a")

	if _, err := service.UpsertDriver(ctx, connect.NewRequest(&cloudv1.UpsertDriverRequest{
		Driver: &cloudv1.DriverRecord{Name: "amqp", ConfigJson: "{\"url\":\"amqp://localhost\"}", Enabled: true},
	})); err != nil {
		t.Fatalf("upsert driver: %v", err)
	}

	listResp, err := service.ListDrivers(ctx, connect.NewRequest(&cloudv1.ListDriversRequest{}))
	if err != nil {
		t.Fatalf("list drivers: %v", err)
	}
	if len(listResp.Msg.GetDrivers()) != 1 {
		t.Fatalf("expected 1 driver")
	}

	if _, err := service.GetDriver(ctx, connect.NewRequest(&cloudv1.GetDriverRequest{Name: "missing"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected not found error")
	}

	if _, err := service.DeleteDriver(ctx, connect.NewRequest(&cloudv1.DeleteDriverRequest{Name: "amqp"})); err != nil {
		t.Fatalf("delete driver: %v", err)
	}
}

func TestRulesServiceCreateUpdateMatch(t *testing.T) {
	ruleStore := storage.NewMockRuleStore()
	driverStore := storage.NewMockDriverStore()
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	driver, err := driverStore.UpsertDriver(ctx, storage.DriverRecord{Name: "amqp", ConfigJSON: "{\"url\":\"amqp://localhost\"}", Enabled: true})
	if err != nil {
		t.Fatalf("upsert driver: %v", err)
	}

	service := &RulesService{Store: ruleStore, DriverStore: driverStore}
	createResp, err := service.CreateRule(ctx, connect.NewRequest(&cloudv1.CreateRuleRequest{
		Rule: &cloudv1.Rule{
			When:        "action == \"opened\"",
			Emit:        []string{"pr.opened"},
			DriverId:    driver.ID,
			TransformJs: "function transform(payload){ return payload; }",
		},
	}))
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	ruleID := createResp.Msg.GetRule().GetId()
	if ruleID == "" {
		t.Fatalf("expected rule id")
	}

	if _, err := service.UpdateRule(ctx, connect.NewRequest(&cloudv1.UpdateRuleRequest{
		Id: ruleID,
		Rule: &cloudv1.Rule{
			When:        "action == \"closed\"",
			Emit:        []string{"pr.closed"},
			DriverId:    driver.ID,
			TransformJs: "function transform(payload){ payload.closed = true; return payload; }",
		},
	})); err != nil {
		t.Fatalf("update rule: %v", err)
	}

	matchResp, err := service.MatchRules(ctx, connect.NewRequest(&cloudv1.MatchRulesRequest{
		Event: &cloudv1.EventPayload{
			Provider: "github",
			Name:     "pull_request",
			Payload:  []byte(`{"action":"closed"}`),
		},
		Rules: []*cloudv1.Rule{
			{When: "action == \"closed\"", Emit: []string{"pr.closed"}, DriverId: driver.ID},
		},
	}))
	if err != nil {
		t.Fatalf("match rules: %v", err)
	}
	if len(matchResp.Msg.GetMatches()) != 1 {
		t.Fatalf("expected 1 match")
	}
}

func TestProvidersServiceLifecycle(t *testing.T) {
	store := storage.NewMockProviderInstanceStore()
	service := &ProvidersService{Store: store}
	ctx := storage.WithTenant(context.Background(), "tenant-a")

	resp, err := service.UpsertProvider(ctx, connect.NewRequest(&cloudv1.UpsertProviderRequest{
		Provider: &cloudv1.ProviderRecord{
			Provider:   "github",
			ConfigJson: "{}",
			Enabled:    true,
		},
	}))
	if err != nil {
		t.Fatalf("upsert provider: %v", err)
	}
	hash := resp.Msg.GetProvider().GetHash()
	if len(hash) != 64 {
		t.Fatalf("expected hash to be generated")
	}

	listResp, err := service.ListProviders(ctx, connect.NewRequest(&cloudv1.ListProvidersRequest{
		Provider: "github",
	}))
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(listResp.Msg.GetProviders()) != 1 {
		t.Fatalf("expected 1 provider")
	}

	if _, err := service.DeleteProvider(ctx, connect.NewRequest(&cloudv1.DeleteProviderRequest{
		Provider: "github",
		Hash:     hash,
	})); err != nil {
		t.Fatalf("delete provider: %v", err)
	}
}

func TestEventLogsServiceLifecycle(t *testing.T) {
	store := storage.NewMockEventLogStore()
	driverStore := storage.NewMockDriverStore()
	ruleStore := storage.NewMockRuleStore()
	replayPublisher := &replayCapturePublisher{}
	service := &EventLogsService{Store: store, RuleStore: ruleStore, DriverStore: driverStore, Publisher: replayPublisher}
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	now := time.Now().UTC()
	httpDriver, err := driverStore.UpsertDriver(ctx, storage.DriverRecord{Name: "http", ConfigJSON: `{"endpoint":"http://localhost:8088/{topic}"}`, Enabled: true})
	if err != nil {
		t.Fatalf("upsert replay driver: %v", err)
	}
	if _, err := ruleStore.CreateRule(ctx, storage.RuleRecord{When: `action == "opened"`, Emit: []string{"relaybus.demo"}, DriverID: httpDriver.ID, TransformJS: `function transform(payload){ payload.replayed = true; return payload; }`}); err != nil {
		t.Fatalf("create replay rule: %v", err)
	}
	if err := store.CreateEventLogs(ctx, []storage.EventLogRecord{
		{ID: "id-1", Provider: "github", Name: "push", RequestID: "req-1", Topic: "relaybus.demo", Body: []byte(`{"action":"opened","a":1}`), TransformedBody: []byte(`{"a":2}`), CreatedAt: now, Matched: true},
		{ID: "id-2", Provider: "gitlab", Name: "merge", RequestID: "req-2", CreatedAt: now.Add(time.Minute)},
	}); err != nil {
		t.Fatalf("create event logs: %v", err)
	}

	listResp, err := service.ListEventLogs(ctx, connect.NewRequest(&cloudv1.ListEventLogsRequest{
		PageSize: 1,
	}))
	if err != nil {
		t.Fatalf("list event logs: %v", err)
	}
	if len(listResp.Msg.GetLogs()) != 1 || listResp.Msg.GetNextPageToken() == "" {
		t.Fatalf("expected paginated results")
	}

	analyticsResp, err := service.GetEventLogAnalytics(ctx, connect.NewRequest(&cloudv1.GetEventLogAnalyticsRequest{}))
	if err != nil {
		t.Fatalf("get analytics: %v", err)
	}
	if analyticsResp.Msg.GetAnalytics().GetTotal() == 0 {
		t.Fatalf("expected analytics totals")
	}

	_, err = service.GetEventLogTimeseries(ctx, connect.NewRequest(&cloudv1.GetEventLogTimeseriesRequest{
		StartTime: timestamppb.New(now.Add(-time.Hour)),
		EndTime:   timestamppb.New(now.Add(time.Hour)),
		Interval:  cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_HOUR,
	}))
	if err != nil {
		t.Fatalf("get timeseries: %v", err)
	}

	_, err = service.GetEventLogBreakdown(ctx, connect.NewRequest(&cloudv1.GetEventLogBreakdownRequest{
		GroupBy: cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_PROVIDER,
	}))
	if err != nil {
		t.Fatalf("get breakdown: %v", err)
	}

	if _, err := service.UpdateEventLogStatus(ctx, connect.NewRequest(&cloudv1.UpdateEventLogStatusRequest{
		LogId:  "id-1",
		Status: "success",
	})); err != nil {
		t.Fatalf("update status: %v", err)
	}

	replayResp, err := service.ReplayEventLog(ctx, connect.NewRequest(&cloudv1.ReplayEventLogRequest{LogId: "id-1"}))
	if err != nil {
		t.Fatalf("replay event log: %v", err)
	}
	if replayResp.Msg.GetLogId() != "id-1" || len(replayResp.Msg.GetResults()) == 0 {
		t.Fatalf("unexpected replay response: %+v", replayResp.Msg)
	}
	if replayPublisher.published != 1 || replayPublisher.lastTopic != "relaybus.demo" || replayPublisher.lastEvent.RawPayload == nil {
		deadline := time.Now().Add(2 * time.Second)
		for replayPublisher.published == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if replayPublisher.published != 1 || replayPublisher.lastTopic != "relaybus.demo" || replayPublisher.lastEvent.RawPayload == nil {
		t.Fatalf("expected replay publish called once asynchronously, got published=%d topic=%s", replayPublisher.published, replayPublisher.lastTopic)
	}
	if !strings.Contains(string(replayPublisher.lastEvent.RawPayload), `"replayed":true`) {
		t.Fatalf("expected replay transform output, got %s", string(replayPublisher.lastEvent.RawPayload))
	}
}

func TestEventLogsServiceReplayValidation(t *testing.T) {
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	store := storage.NewMockEventLogStore()
	driverStore := storage.NewMockDriverStore()
	ruleStore := storage.NewMockRuleStore()
	service := &EventLogsService{Store: store, RuleStore: ruleStore, DriverStore: driverStore, Publisher: &replayCapturePublisher{}}

	if _, err := service.ReplayEventLog(ctx, connect.NewRequest(&cloudv1.ReplayEventLogRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("expected invalid argument for empty replay request")
	}

	if _, err := service.ReplayEventLog(ctx, connect.NewRequest(&cloudv1.ReplayEventLogRequest{LogId: "missing"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("expected not found for missing log")
	}
}

type replayCapturePublisher struct {
	published int
	lastTopic string
	lastEvent core.Event
}

func (r *replayCapturePublisher) Publish(ctx context.Context, topic string, event core.Event) error {
	r.published++
	r.lastTopic = topic
	r.lastEvent = event
	return nil
}

func (r *replayCapturePublisher) PublishForDrivers(ctx context.Context, topic string, event core.Event, drivers []string) error {
	return r.Publish(ctx, topic, event)
}

func (r *replayCapturePublisher) Close() error { return nil }

func TestNamespacesServiceList(t *testing.T) {
	store := storage.NewMockNamespaceStore()
	service := &NamespacesService{Store: store}
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	if err := store.UpsertNamespace(ctx, storage.NamespaceRecord{
		Provider: "github",
		RepoID:   "1",
		Owner:    "org",
		RepoName: "repo",
		FullName: "org/repo",
	}); err != nil {
		t.Fatalf("upsert namespace: %v", err)
	}
	resp, err := service.ListNamespaces(ctx, connect.NewRequest(&cloudv1.ListNamespacesRequest{
		Provider: "github",
		Owner:    "org",
		Repo:     "repo",
	}))
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if len(resp.Msg.GetNamespaces()) != 1 {
		t.Fatalf("expected 1 namespace")
	}
}

func TestRuleHelpersAndPagination(t *testing.T) {
	if _, _, _, _, err := parseRuleInput(nil); err == nil {
		t.Fatalf("expected missing rule error")
	}
	when, emit, driverID, transformJS, err := parseRuleInput(&cloudv1.Rule{
		When:        " when ",
		Emit:        []string{"topic"},
		DriverId:    " driver ",
		TransformJs: " function transform(payload){ return payload; } ",
	})
	if err != nil || when != "when" || emit[0] != "topic" || driverID != "driver" || transformJS == "" {
		t.Fatalf("unexpected parse output")
	}

	normalized, err := normalizeCoreRule("  action == \"opened\"  ", []string{"topic"}, "amqp", "amqp")
	if err != nil || strings.TrimSpace(normalized.When) == "" {
		t.Fatalf("expected normalized rule")
	}

	if _, err := decodePageToken("bad"); err == nil {
		t.Fatalf("expected invalid page token error")
	}
	if token := encodePageToken(0); token != "" {
		t.Fatalf("expected empty token")
	}
	if token := encodePageTokenFromRaw("5"); token == "" {
		t.Fatalf("expected encoded token")
	}
	if _, _, _, _, err := parseRuleInput(&cloudv1.Rule{
		When:     "when",
		Emit:     []string{"one", "two"},
		DriverId: "driver",
	}); err == nil {
		t.Fatalf("expected emit count validation error")
	}
}

func TestConnectHelperCoverage(t *testing.T) {
	t.Run("logError handles nil and logger", func(t *testing.T) {
		logError(nil, "ignored", nil)

		var buf bytes.Buffer
		logger := log.New(&buf, "", 0)
		logError(logger, "boom", context.Canceled)
		if !strings.Contains(buf.String(), "boom") || !strings.Contains(buf.String(), "context canceled") {
			t.Fatalf("unexpected log output: %q", buf.String())
		}
	})

	t.Run("providerConfigFromAuthConfig selects provider", func(t *testing.T) {
		cfg := auth.Config{
			GitHub:    auth.ProviderConfig{Key: "gh"},
			GitLab:    auth.ProviderConfig{Key: "gl"},
			Bitbucket: auth.ProviderConfig{Key: "bb"},
			Slack:     auth.ProviderConfig{Key: "sl"},
			Atlassian: auth.ProviderConfig{Key: "at"},
			Jira:      auth.ProviderConfig{Key: "jr"},
		}

		if got := providerConfigFromAuthConfig(cfg, "github"); got.Key != "gh" {
			t.Fatalf("expected github config")
		}
		if got := providerConfigFromAuthConfig(cfg, "gitlab"); got.Key != "gl" {
			t.Fatalf("expected gitlab config")
		}
		if got := providerConfigFromAuthConfig(cfg, "bitbucket"); got.Key != "bb" {
			t.Fatalf("expected bitbucket config")
		}
		if got := providerConfigFromAuthConfig(cfg, "slack"); got.Key != "sl" {
			t.Fatalf("expected slack config")
		}
		if got := providerConfigFromAuthConfig(cfg, "atlassian"); got.Key != "at" {
			t.Fatalf("expected atlassian config")
		}
		if got := providerConfigFromAuthConfig(cfg, "jira"); got.Key != "at" {
			t.Fatalf("expected jira alias to resolve atlassian config")
		}
		if got := providerConfigFromAuthConfig(cfg, "unknown"); got.Key != "gh" {
			t.Fatalf("expected fallback github config")
		}
	})

	t.Run("toProtoRuleRecords converts list", func(t *testing.T) {
		if got := toProtoRuleRecords(nil); len(got) != 0 {
			t.Fatalf("expected empty conversion")
		}

		now := time.Now().UTC()
		records := []storage.RuleRecord{{
			ID:               "rule-1",
			When:             `action == "opened"`,
			Emit:             []string{"pr.opened"},
			DriverID:         "driver-1",
			TransformJS:      "function transform(payload){ return payload; }",
			DriverName:       "amqp",
			DriverConfigJSON: `{"url":"amqp://localhost"}`,
			DriverEnabled:    true,
			CreatedAt:        now,
			UpdatedAt:        now,
		}}

		got := toProtoRuleRecords(records)
		if len(got) != 1 {
			t.Fatalf("expected 1 converted rule, got %d", len(got))
		}
		if got[0].GetId() != "rule-1" || got[0].GetDriverId() != "driver-1" {
			t.Fatalf("unexpected converted rule: %+v", got[0])
		}
		if got[0].GetWhen() == "" || len(got[0].GetEmit()) != 1 || got[0].GetEmit()[0] != "pr.opened" {
			t.Fatalf("unexpected converted rule payload: %+v", got[0])
		}
		if got[0].GetTransformJs() == "" {
			t.Fatalf("expected transform_js to be converted")
		}
	})

	t.Run("event log enum converters", func(t *testing.T) {
		intervalCases := []cloudv1.EventLogTimeseriesInterval{
			cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_HOUR,
			cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_DAY,
			cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_WEEK,
		}
		for _, in := range intervalCases {
			if _, err := eventLogIntervalFromProto(in); err != nil {
				t.Fatalf("interval conversion failed for %v: %v", in, err)
			}
		}
		if _, err := eventLogIntervalFromProto(cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_UNSPECIFIED); err == nil {
			t.Fatalf("expected invalid interval error")
		}

		groupCases := []cloudv1.EventLogBreakdownGroup{
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_PROVIDER,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_EVENT,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_RULE_ID,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_RULE_WHEN,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_TOPIC,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_NAMESPACE_ID,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_NAMESPACE_NAME,
			cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_INSTALLATION_ID,
		}
		for _, in := range groupCases {
			if _, err := eventLogBreakdownGroupFromProto(in); err != nil {
				t.Fatalf("group conversion failed for %v: %v", in, err)
			}
		}
		if _, err := eventLogBreakdownGroupFromProto(cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_UNSPECIFIED); err == nil {
			t.Fatalf("expected invalid group error")
		}
	})
}
