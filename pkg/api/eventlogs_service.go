package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/google/uuid"
	"github.com/relaymesh/relaymesh/pkg/core"
	driverspkg "github.com/relaymesh/relaymesh/pkg/drivers"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/storage"
	"github.com/relaymesh/relaymesh/pkg/webhook"
)

const (
	defaultEventLogPageSize = 50
	maxEventLogPageSize     = 200
)

// EventLogsService handles queries for webhook event logs and analytics.
type EventLogsService struct {
	Store       storage.EventLogStore
	RuleStore   storage.RuleStore
	DriverStore storage.DriverStore
	Publisher   core.Publisher
	RulesStrict bool
	Logger      *log.Logger
}

func (s *EventLogsService) ListEventLogs(
	ctx context.Context,
	req *connect.Request[cloudv1.ListEventLogsRequest],
) (*connect.Response[cloudv1.ListEventLogsResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	pageSize := int(req.Msg.GetPageSize())
	if pageSize <= 0 {
		pageSize = defaultEventLogPageSize
	}
	if pageSize > maxEventLogPageSize {
		pageSize = maxEventLogPageSize
	}
	offset, err := decodePageToken(req.Msg.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var matched *bool
	if req.Msg.GetMatchedOnly() {
		value := true
		matched = &value
	}
	filter := storage.EventLogFilter{
		Provider:       strings.TrimSpace(req.Msg.GetProvider()),
		Name:           strings.TrimSpace(req.Msg.GetName()),
		Topic:          strings.TrimSpace(req.Msg.GetTopic()),
		RequestID:      strings.TrimSpace(req.Msg.GetRequestId()),
		StateID:        strings.TrimSpace(req.Msg.GetStateId()),
		InstallationID: strings.TrimSpace(req.Msg.GetInstallationId()),
		NamespaceID:    strings.TrimSpace(req.Msg.GetNamespaceId()),
		NamespaceName:  strings.TrimSpace(req.Msg.GetNamespaceName()),
		RuleID:         strings.TrimSpace(req.Msg.GetRuleId()),
		RuleWhen:       strings.TrimSpace(req.Msg.GetRuleWhen()),
		Matched:        matched,
		StartTime:      fromProtoTimestamp(req.Msg.GetStartTime()),
		EndTime:        fromProtoTimestamp(req.Msg.GetEndTime()),
		Limit:          pageSize + 1,
		Offset:         offset,
	}
	records, err := s.Store.ListEventLogs(ctx, filter)
	if err != nil {
		logError(s.Logger, "list event logs failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list event logs failed"))
	}
	logger := s.Logger
	if logger == nil {
		logger = log.Default()
	}
	headerTenant := ""
	if req != nil {
		header := req.Header()
		if header != nil {
			headerTenant = strings.TrimSpace(header.Get("X-Tenant-ID"))
			if headerTenant == "" {
				headerTenant = strings.TrimSpace(header.Get("X-Githooks-Tenant-ID"))
			}
		}
	}
	resolvedTenant := storage.TenantFromContext(ctx)
	if len(records) > 0 {
		logger.Printf("event log list tenant=%s header_tenant=%s request_id=%s count=%d first_log_id=%s first_status=%s", resolvedTenant, headerTenant, filter.RequestID, len(records), records[0].ID, records[0].Status)
	} else {
		logger.Printf("event log list tenant=%s header_tenant=%s request_id=%s count=0", resolvedTenant, headerTenant, filter.RequestID)
	}
	nextToken := ""
	if len(records) > pageSize {
		records = records[:pageSize]
		nextToken = encodePageToken(offset + pageSize)
	}
	resp := &cloudv1.ListEventLogsResponse{
		Logs:          toProtoEventLogRecords(records),
		NextPageToken: nextToken,
	}
	return connect.NewResponse(resp), nil
}

func (s *EventLogsService) GetEventLogAnalytics(
	ctx context.Context,
	req *connect.Request[cloudv1.GetEventLogAnalyticsRequest],
) (*connect.Response[cloudv1.GetEventLogAnalyticsResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	var matched *bool
	if req.Msg.GetMatchedOnly() {
		value := true
		matched = &value
	}
	filter := storage.EventLogFilter{
		Provider:       strings.TrimSpace(req.Msg.GetProvider()),
		Name:           strings.TrimSpace(req.Msg.GetName()),
		Topic:          strings.TrimSpace(req.Msg.GetTopic()),
		RequestID:      strings.TrimSpace(req.Msg.GetRequestId()),
		StateID:        strings.TrimSpace(req.Msg.GetStateId()),
		InstallationID: strings.TrimSpace(req.Msg.GetInstallationId()),
		NamespaceID:    strings.TrimSpace(req.Msg.GetNamespaceId()),
		NamespaceName:  strings.TrimSpace(req.Msg.GetNamespaceName()),
		RuleID:         strings.TrimSpace(req.Msg.GetRuleId()),
		RuleWhen:       strings.TrimSpace(req.Msg.GetRuleWhen()),
		Matched:        matched,
		StartTime:      fromProtoTimestamp(req.Msg.GetStartTime()),
		EndTime:        fromProtoTimestamp(req.Msg.GetEndTime()),
	}
	analytics, err := s.Store.GetEventLogAnalytics(ctx, filter)
	if err != nil {
		logError(s.Logger, "event log analytics failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("event log analytics failed"))
	}
	resp := &cloudv1.GetEventLogAnalyticsResponse{
		Analytics: toProtoEventLogAnalytics(analytics),
	}
	return connect.NewResponse(resp), nil
}

func (s *EventLogsService) GetEventLogTimeseries(
	ctx context.Context,
	req *connect.Request[cloudv1.GetEventLogTimeseriesRequest],
) (*connect.Response[cloudv1.GetEventLogTimeseriesResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	interval, err := eventLogIntervalFromProto(req.Msg.GetInterval())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var matched *bool
	if req.Msg.GetMatchedOnly() {
		value := true
		matched = &value
	}
	filter := storage.EventLogFilter{
		Provider:       strings.TrimSpace(req.Msg.GetProvider()),
		Name:           strings.TrimSpace(req.Msg.GetName()),
		Topic:          strings.TrimSpace(req.Msg.GetTopic()),
		RequestID:      strings.TrimSpace(req.Msg.GetRequestId()),
		StateID:        strings.TrimSpace(req.Msg.GetStateId()),
		InstallationID: strings.TrimSpace(req.Msg.GetInstallationId()),
		NamespaceID:    strings.TrimSpace(req.Msg.GetNamespaceId()),
		NamespaceName:  strings.TrimSpace(req.Msg.GetNamespaceName()),
		RuleID:         strings.TrimSpace(req.Msg.GetRuleId()),
		RuleWhen:       strings.TrimSpace(req.Msg.GetRuleWhen()),
		Matched:        matched,
		StartTime:      fromProtoTimestamp(req.Msg.GetStartTime()),
		EndTime:        fromProtoTimestamp(req.Msg.GetEndTime()),
	}
	buckets, err := s.Store.GetEventLogTimeseries(ctx, filter, interval)
	if err != nil {
		logError(s.Logger, "event log timeseries failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("event log timeseries failed"))
	}
	resp := &cloudv1.GetEventLogTimeseriesResponse{
		Buckets: toProtoEventLogTimeseries(buckets),
	}
	return connect.NewResponse(resp), nil
}

func (s *EventLogsService) GetEventLogBreakdown(
	ctx context.Context,
	req *connect.Request[cloudv1.GetEventLogBreakdownRequest],
) (*connect.Response[cloudv1.GetEventLogBreakdownResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	groupBy, err := eventLogBreakdownGroupFromProto(req.Msg.GetGroupBy())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	sortBy := eventLogBreakdownSortFromProto(req.Msg.GetSortBy())
	pageSize := int(req.Msg.GetPageSize())
	if pageSize <= 0 {
		pageSize = defaultEventLogPageSize
	}
	if pageSize > maxEventLogPageSize {
		pageSize = maxEventLogPageSize
	}
	offset, err := decodePageToken(req.Msg.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var matched *bool
	if req.Msg.GetMatchedOnly() {
		value := true
		matched = &value
	}
	filter := storage.EventLogFilter{
		Provider:       strings.TrimSpace(req.Msg.GetProvider()),
		Name:           strings.TrimSpace(req.Msg.GetName()),
		Topic:          strings.TrimSpace(req.Msg.GetTopic()),
		RequestID:      strings.TrimSpace(req.Msg.GetRequestId()),
		StateID:        strings.TrimSpace(req.Msg.GetStateId()),
		InstallationID: strings.TrimSpace(req.Msg.GetInstallationId()),
		NamespaceID:    strings.TrimSpace(req.Msg.GetNamespaceId()),
		NamespaceName:  strings.TrimSpace(req.Msg.GetNamespaceName()),
		RuleID:         strings.TrimSpace(req.Msg.GetRuleId()),
		RuleWhen:       strings.TrimSpace(req.Msg.GetRuleWhen()),
		Matched:        matched,
		StartTime:      fromProtoTimestamp(req.Msg.GetStartTime()),
		EndTime:        fromProtoTimestamp(req.Msg.GetEndTime()),
	}
	breakdowns, nextToken, err := s.Store.GetEventLogBreakdown(
		ctx,
		filter,
		groupBy,
		sortBy,
		req.Msg.GetSortDesc(),
		pageSize,
		strconv.Itoa(offset),
		req.Msg.GetIncludeLatency(),
	)
	if err != nil {
		logError(s.Logger, "event log breakdown failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("event log breakdown failed"))
	}
	resp := &cloudv1.GetEventLogBreakdownResponse{
		Breakdowns:    toProtoEventLogBreakdowns(breakdowns),
		NextPageToken: encodePageTokenFromRaw(nextToken),
	}
	return connect.NewResponse(resp), nil
}

func (s *EventLogsService) UpdateEventLogStatus(
	ctx context.Context,
	req *connect.Request[cloudv1.UpdateEventLogStatusRequest],
) (*connect.Response[cloudv1.UpdateEventLogStatusResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request is required"))
	}
	msg := req.Msg
	logID := strings.TrimSpace(msg.GetLogId())
	status := strings.TrimSpace(msg.GetStatus())
	errMsg := strings.TrimSpace(msg.GetErrorMessage())
	tenantID := storage.TenantFromContext(ctx)
	logger := s.Logger
	if logger == nil {
		logger = log.Default()
	}
	var headerTenant string
	header := req.Header()
	if header != nil {
		headerTenant = strings.TrimSpace(header.Get("X-Tenant-ID"))
		if headerTenant == "" {
			headerTenant = strings.TrimSpace(header.Get("X-Githooks-Tenant-ID"))
		}
	}
	logger.Printf("event log update request log_id=%s status=%s tenant=%s header_tenant=%s err_len=%d", logID, status, tenantID, headerTenant, len(errMsg))
	if err := s.Store.UpdateEventLogStatus(ctx, logID, status, errMsg); err != nil {
		logError(s.Logger, "event log update failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("event log update failed"))
	}
	logger.Printf("event log update ok log_id=%s status=%s tenant=%s", logID, status, tenantID)
	return connect.NewResponse(&cloudv1.UpdateEventLogStatusResponse{}), nil
}

func (s *EventLogsService) ReplayEventLog(
	ctx context.Context,
	req *connect.Request[cloudv1.ReplayEventLogRequest],
) (*connect.Response[cloudv1.ReplayEventLogResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	if s.DriverStore == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("driver storage not configured"))
	}
	if s.RuleStore == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("rule storage not configured"))
	}
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request is required"))
	}
	logID := strings.TrimSpace(req.Msg.GetLogId())
	if logID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("log_id is required"))
	}

	record, err := s.Store.GetEventLog(ctx, logID)
	if err != nil {
		logError(s.Logger, "get event log failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get event log failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("event log not found"))
	}
	event, err := webhook.BuildReplayEvent(*record)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if strings.TrimSpace(event.TenantID) == "" {
		event.TenantID = storage.TenantFromContext(ctx)
	}

	rules := webhook.MatchRulesForEvent(ctx, event, storage.TenantFromContext(ctx), s.RuleStore, s.DriverStore, s.RulesStrict, s.Logger)
	if len(rules) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no matching rules for event log"))
	}
	ruleMatches := webhook.RuleMatchesFromMatchedRules(rules)
	if len(ruleMatches) == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no matched topics for event log"))
	}
	replayRecords := buildReplayEventLogRecords(event, ruleMatches)
	if err := s.Store.CreateEventLogs(ctx, replayRecords); err != nil {
		logError(s.Logger, "replay event log write failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("replay event log write failed"))
	}

	publisher := s.Publisher
	publishers := map[string]core.Publisher{}

	results := make([]*cloudv1.ReplayPublishResult, 0, len(ruleMatches))
	for idx, rule := range ruleMatches {
		driverName := strings.TrimSpace(rule.DriverName)
		if driverName == "" {
			driverName = strings.TrimSpace(rule.DriverID)
		}
		results = append(results, &cloudv1.ReplayPublishResult{RuleId: rule.RuleID, Topic: rule.Topic, DriverName: driverName, Status: "queued"})
		_ = idx
	}

	tenantCtx := storage.WithTenant(context.Background(), storage.TenantFromContext(ctx))
	go s.runReplayAsync(tenantCtx, event, ruleMatches, replayRecords, publisher, publishers)

	return connect.NewResponse(&cloudv1.ReplayEventLogResponse{
		LogId:   record.ID,
		Results: results,
	}), nil
}

func (s *EventLogsService) runReplayAsync(ctx context.Context, event core.Event, matches []core.RuleMatch, records []storage.EventLogRecord, sharedPublisher core.Publisher, publishers map[string]core.Publisher) {
	if s == nil || s.Store == nil {
		return
	}
	defer func() {
		if sharedPublisher != nil {
			return
		}
		for _, pub := range publishers {
			if pub == nil {
				continue
			}
			_ = pub.Close()
		}
	}()

	for idx, rule := range matches {
		if idx >= len(records) {
			continue
		}
		replayLogID := records[idx].ID
		driverRecord, err := s.replayResolveDriverRecord(ctx, rule)
		if err != nil {
			_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "failed", err.Error())
			continue
		}
		transformed, err := webhook.ApplyRuleTransform(event, rule.TransformJS)
		if err != nil {
			_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "failed", err.Error())
			continue
		}
		_ = s.Store.UpdateEventLogTransformedPayload(ctx, replayLogID, transformed.RawPayload)

		pub := sharedPublisher
		if pub == nil {
			name := strings.TrimSpace(driverRecord.Name)
			if existing, ok := publishers[name]; ok {
				pub = existing
			} else {
				cfg, cfgErr := driverspkg.ConfigFromDriver(driverRecord.Name, driverRecord.ConfigJSON)
				if cfgErr != nil {
					_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "failed", cfgErr.Error())
					continue
				}
				created, createErr := core.NewPublisher(cfg)
				if createErr != nil {
					_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "failed", createErr.Error())
					continue
				}
				publishers[name] = created
				pub = created
			}
		}
		publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = pub.PublishForDrivers(publishCtx, rule.Topic, transformed, []string{driverRecord.Name})
		cancel()
		if err != nil {
			_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "failed", err.Error())
			continue
		}
		_ = s.Store.UpdateEventLogStatus(ctx, replayLogID, "success", "")
	}
}

func buildReplayEventLogRecords(event core.Event, matches []core.RuleMatch) []storage.EventLogRecord {
	body := append([]byte(nil), event.RawPayload...)
	bodyHash := ""
	if len(body) > 0 {
		bodyHash = webhookHashBody(body)
	}
	records := make([]storage.EventLogRecord, 0, len(matches))
	for _, match := range matches {
		driverName := strings.TrimSpace(match.DriverName)
		if driverName == "" {
			driverName = strings.TrimSpace(match.DriverID)
		}
		records = append(records, storage.EventLogRecord{
			ID:             uuid.NewString(),
			Provider:       event.Provider,
			Name:           event.Name,
			RequestID:      event.RequestID,
			StateID:        event.StateID,
			InstallationID: event.InstallationID,
			NamespaceID:    event.NamespaceID,
			NamespaceName:  event.NamespaceName,
			Topic:          match.Topic,
			RuleID:         match.RuleID,
			RuleWhen:       match.RuleWhen,
			Drivers:        []string{driverName},
			Headers:        cloneReplayHeaders(event.Headers),
			Body:           body,
			BodyHash:       bodyHash,
			Status:         "queued",
			Matched:        true,
		})
	}
	return records
}

func webhookHashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func cloneReplayHeaders(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		if len(values) == 0 {
			continue
		}
		out[key] = append([]string(nil), values...)
	}
	return out
}

func (s *EventLogsService) replayResolveDriverRecord(ctx context.Context, match core.RuleMatch) (*storage.DriverRecord, error) {
	name := strings.TrimSpace(match.DriverName)
	if name != "" {
		record, err := s.DriverStore.GetDriver(ctx, name)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, errors.New("driver not found")
		}
		if !record.Enabled {
			return nil, errors.New("driver is disabled")
		}
		return record, nil
	}
	driverID := strings.TrimSpace(match.DriverID)
	if driverID == "" {
		return nil, errors.New("driver_id is required")
	}
	record, err := s.DriverStore.GetDriverByID(ctx, driverID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("driver not found")
	}
	if !record.Enabled {
		return nil, errors.New("driver is disabled")
	}
	return record, nil
}

func decodePageToken(token string) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, errors.New("invalid page token")
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid page token")
	}
	return offset, nil
}

func encodePageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func encodePageTokenFromRaw(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset <= 0 {
		return ""
	}
	return encodePageToken(offset)
}

func eventLogIntervalFromProto(interval cloudv1.EventLogTimeseriesInterval) (storage.EventLogInterval, error) {
	switch interval {
	case cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_HOUR:
		return storage.EventLogIntervalHour, nil
	case cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_DAY:
		return storage.EventLogIntervalDay, nil
	case cloudv1.EventLogTimeseriesInterval_EVENT_LOG_TIMESERIES_INTERVAL_WEEK:
		return storage.EventLogIntervalWeek, nil
	default:
		return "", errors.New("invalid interval")
	}
}

func eventLogBreakdownGroupFromProto(group cloudv1.EventLogBreakdownGroup) (storage.EventLogBreakdownGroup, error) {
	switch group {
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_PROVIDER:
		return storage.EventLogBreakdownProvider, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_EVENT:
		return storage.EventLogBreakdownEvent, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_RULE_ID:
		return storage.EventLogBreakdownRuleID, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_RULE_WHEN:
		return storage.EventLogBreakdownRuleWhen, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_TOPIC:
		return storage.EventLogBreakdownTopic, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_NAMESPACE_ID:
		return storage.EventLogBreakdownNamespaceID, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_NAMESPACE_NAME:
		return storage.EventLogBreakdownNamespaceName, nil
	case cloudv1.EventLogBreakdownGroup_EVENT_LOG_BREAKDOWN_GROUP_INSTALLATION_ID:
		return storage.EventLogBreakdownInstallation, nil
	default:
		return "", errors.New("invalid group_by")
	}
}

func eventLogBreakdownSortFromProto(sortBy cloudv1.EventLogBreakdownSort) storage.EventLogBreakdownSort {
	switch sortBy {
	case cloudv1.EventLogBreakdownSort_EVENT_LOG_BREAKDOWN_SORT_MATCHED:
		return storage.EventLogBreakdownSortMatched
	case cloudv1.EventLogBreakdownSort_EVENT_LOG_BREAKDOWN_SORT_FAILED:
		return storage.EventLogBreakdownSortFailed
	default:
		return storage.EventLogBreakdownSortCount
	}
}
