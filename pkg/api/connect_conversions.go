package api

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/relaymesh/relaymesh/pkg/core"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func toProtoInstallations(records []storage.InstallRecord) []*cloudv1.InstallRecord {
	out := make([]*cloudv1.InstallRecord, 0, len(records))
	for _, record := range records {
		out = append(out, toProtoInstallation(record))
	}
	return out
}

func toProtoInstallation(record storage.InstallRecord) *cloudv1.InstallRecord {
	return &cloudv1.InstallRecord{
		Provider:            record.Provider,
		AccountId:           record.AccountID,
		AccountName:         record.AccountName,
		InstallationId:      record.InstallationID,
		AccessToken:         record.AccessToken,
		RefreshToken:        record.RefreshToken,
		ExpiresAt:           toProtoTimestampPtr(record.ExpiresAt),
		MetadataJson:        record.MetadataJSON,
		CreatedAt:           toProtoTimestamp(record.CreatedAt),
		UpdatedAt:           toProtoTimestamp(record.UpdatedAt),
		ProviderInstanceKey: record.ProviderInstanceKey,
		EnterpriseId:        record.EnterpriseID,
		EnterpriseSlug:      record.EnterpriseSlug,
		EnterpriseName:      record.EnterpriseName,
	}
}

func toProtoNamespaces(records []storage.NamespaceRecord) []*cloudv1.NamespaceRecord {
	out := make([]*cloudv1.NamespaceRecord, 0, len(records))
	for _, record := range records {
		out = append(out, toProtoNamespace(record))
	}
	return out
}

func toProtoNamespace(record storage.NamespaceRecord) *cloudv1.NamespaceRecord {
	return &cloudv1.NamespaceRecord{
		Provider:        record.Provider,
		RepoId:          record.RepoID,
		AccountId:       record.AccountID,
		InstallationId:  record.InstallationID,
		Owner:           record.Owner,
		RepoName:        record.RepoName,
		FullName:        record.FullName,
		Visibility:      record.Visibility,
		DefaultBranch:   record.DefaultBranch,
		HttpUrl:         record.HTTPURL,
		SshUrl:          record.SSHURL,
		WebhooksEnabled: record.WebhooksEnabled,
		CreatedAt:       toProtoTimestamp(record.CreatedAt),
		UpdatedAt:       toProtoTimestamp(record.UpdatedAt),
	}
}

func toProtoTimestamp(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value)
}

func toProtoTimestampPtr(value *time.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(*value)
}

func fromProtoTimestamp(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	parsed := value.AsTime()
	if parsed.IsZero() {
		return time.Time{}
	}
	return parsed
}

func fromProtoTimestampPtr(value *timestamppb.Timestamp) *time.Time {
	if value == nil {
		return nil
	}
	parsed := value.AsTime()
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func toProtoRuleMatches(matches []core.MatchedRule) []*cloudv1.RuleMatch {
	out := make([]*cloudv1.RuleMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, &cloudv1.RuleMatch{
			When:        match.When,
			Emit:        append([]string(nil), match.Emit...),
			DriverId:    match.DriverID,
			TransformJs: match.TransformJS,
		})
	}
	return out
}

func toProtoDriverRecord(record *storage.DriverRecord) *cloudv1.DriverRecord {
	if record == nil {
		return nil
	}
	return &cloudv1.DriverRecord{
		Id:         record.ID,
		Name:       record.Name,
		ConfigJson: record.ConfigJSON,
		Enabled:    record.Enabled,
		CreatedAt:  toProtoTimestamp(record.CreatedAt),
		UpdatedAt:  toProtoTimestamp(record.UpdatedAt),
	}
}

func toProtoDriverRecords(records []storage.DriverRecord) []*cloudv1.DriverRecord {
	out := make([]*cloudv1.DriverRecord, 0, len(records))
	for _, record := range records {
		item := record
		out = append(out, toProtoDriverRecord(&item))
	}
	return out
}

func toProtoRuleRecords(records []storage.RuleRecord) []*cloudv1.RuleRecord {
	out := make([]*cloudv1.RuleRecord, 0, len(records))
	for _, record := range records {
		out = append(out, toProtoRuleRecord(record))
	}
	return out
}

func toProtoRuleRecord(record storage.RuleRecord) *cloudv1.RuleRecord {
	return &cloudv1.RuleRecord{
		Id:          record.ID,
		When:        record.When,
		Emit:        append([]string(nil), record.Emit...),
		DriverId:    record.DriverID,
		TransformJs: record.TransformJS,
		CreatedAt:   toProtoTimestamp(record.CreatedAt),
		UpdatedAt:   toProtoTimestamp(record.UpdatedAt),
	}
}

func toProtoEventLogRecords(records []storage.EventLogRecord) []*cloudv1.EventLogRecord {
	out := make([]*cloudv1.EventLogRecord, 0, len(records))
	for _, record := range records {
		out = append(out, toProtoEventLogRecord(record))
	}
	return out
}

func toProtoEventLogRecord(record storage.EventLogRecord) *cloudv1.EventLogRecord {
	return &cloudv1.EventLogRecord{
		Id:              record.ID,
		Provider:        record.Provider,
		Name:            record.Name,
		RequestId:       record.RequestID,
		StateId:         record.StateID,
		InstallationId:  record.InstallationID,
		NamespaceId:     record.NamespaceID,
		NamespaceName:   record.NamespaceName,
		Topic:           record.Topic,
		RuleId:          record.RuleID,
		RuleWhen:        record.RuleWhen,
		Drivers:         append([]string(nil), record.Drivers...),
		Matched:         record.Matched,
		Status:          record.Status,
		ErrorMessage:    record.ErrorMessage,
		CreatedAt:       toProtoTimestamp(record.CreatedAt),
		UpdatedAt:       toProtoTimestamp(record.UpdatedAt),
		Headers:         toProtoEventLogHeaders(record.Headers),
		Body:            append([]byte(nil), record.Body...),
		BodyHash:        record.BodyHash,
		TransformedBody: append([]byte(nil), record.TransformedBody...),
	}
}

func toProtoEventLogHeaders(headers map[string][]string) map[string]*cloudv1.EventLogHeaderValues {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]*cloudv1.EventLogHeaderValues, len(headers))
	for key, values := range headers {
		out[key] = &cloudv1.EventLogHeaderValues{
			Values: append([]string(nil), values...),
		}
	}
	return out
}

func toProtoEventLogAnalytics(analytics storage.EventLogAnalytics) *cloudv1.EventLogAnalytics {
	return &cloudv1.EventLogAnalytics{
		Total:            analytics.Total,
		Matched:          analytics.Matched,
		DistinctRequests: analytics.DistinctReq,
		ByProvider:       toProtoEventLogCounts(analytics.ByProvider),
		ByEvent:          toProtoEventLogCounts(analytics.ByEvent),
		ByTopic:          toProtoEventLogCounts(analytics.ByTopic),
		ByRule:           toProtoEventLogCounts(analytics.ByRule),
		ByInstallation:   toProtoEventLogCounts(analytics.ByInstall),
		ByNamespace:      toProtoEventLogCounts(analytics.ByNamespace),
	}
}

func toProtoEventLogCounts(counts []storage.EventLogCount) []*cloudv1.EventLogCount {
	out := make([]*cloudv1.EventLogCount, 0, len(counts))
	for _, count := range counts {
		out = append(out, &cloudv1.EventLogCount{
			Key:   count.Key,
			Count: count.Count,
		})
	}
	return out
}

func toProtoEventLogTimeseries(buckets []storage.EventLogTimeseriesBucket) []*cloudv1.EventLogTimeseriesBucket {
	out := make([]*cloudv1.EventLogTimeseriesBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, &cloudv1.EventLogTimeseriesBucket{
			StartTime:        toProtoTimestamp(bucket.Start),
			EndTime:          toProtoTimestamp(bucket.End),
			EventCount:       bucket.EventCount,
			MatchedCount:     bucket.MatchedCount,
			DistinctRequests: bucket.DistinctReq,
			FailedCount:      bucket.FailureCount,
		})
	}
	return out
}

func toProtoEventLogBreakdowns(breakdowns []storage.EventLogBreakdown) []*cloudv1.EventLogBreakdown {
	out := make([]*cloudv1.EventLogBreakdown, 0, len(breakdowns))
	for _, item := range breakdowns {
		out = append(out, &cloudv1.EventLogBreakdown{
			Key:          item.Key,
			EventCount:   item.EventCount,
			MatchedCount: item.MatchedCount,
			FailedCount:  item.FailureCount,
			LatencyP50Ms: item.LatencyP50MS,
			LatencyP95Ms: item.LatencyP95MS,
			LatencyP99Ms: item.LatencyP99MS,
		})
	}
	return out
}

func toProtoProviderRecord(record *storage.ProviderInstanceRecord) *cloudv1.ProviderRecord {
	if record == nil {
		return nil
	}
	return &cloudv1.ProviderRecord{
		Provider:        record.Provider,
		Hash:            record.Key,
		ConfigJson:      record.ConfigJSON,
		Enabled:         record.Enabled,
		RedirectBaseUrl: record.RedirectBaseURL,
		CreatedAt:       timestamppb.New(record.CreatedAt),
		UpdatedAt:       timestamppb.New(record.UpdatedAt),
	}
}

func toProtoProviderRecords(records []storage.ProviderInstanceRecord) []*cloudv1.ProviderRecord {
	out := make([]*cloudv1.ProviderRecord, 0, len(records))
	for _, record := range records {
		item := record
		out = append(out, toProtoProviderRecord(&item))
	}
	return out
}
