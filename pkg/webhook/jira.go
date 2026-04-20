package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

type JiraHandler struct {
	secret         string
	rules          *core.RuleEngine
	publisher      core.Publisher
	logger         *log.Logger
	maxBody        int64
	debugEvents    bool
	installStore   storage.Store
	logs           storage.EventLogStore
	ruleStore      storage.RuleStore
	driverStore    storage.DriverStore
	rulesStrict    bool
	dynamicDrivers *drivers.DynamicPublisherCache
}

func NewJiraHandler(secret string, rules *core.RuleEngine, publisher core.Publisher, logger *log.Logger, maxBody int64, debugEvents bool, installStore storage.Store, logs storage.EventLogStore, ruleStore storage.RuleStore, driverStore storage.DriverStore, rulesStrict bool, dynamicDrivers *drivers.DynamicPublisherCache) (*JiraHandler, error) {
	if logger == nil {
		logger = log.Default()
	}
	return &JiraHandler{
		secret:         strings.TrimSpace(secret),
		rules:          rules,
		publisher:      publisher,
		logger:         logger,
		maxBody:        maxBody,
		debugEvents:    debugEvents,
		installStore:   installStore,
		logs:           logs,
		ruleStore:      ruleStore,
		driverStore:    driverStore,
		rulesStrict:    rulesStrict,
		dynamicDrivers: dynamicDrivers,
	}, nil
}

func (h *JiraHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r, logger, reqID, rawBody, ok := prepareWebhookRequest(w, r, h.maxBody, h.logger)
	if !ok {
		return
	}

	if err := h.verifySignature(r.Header, rawBody); err != nil {
		logger.Printf("jira signature verification failed: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	rawObject, data := rawObjectAndFlatten(rawBody)
	if rawObject == nil {
		logger.Printf("jira payload invalid json")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventName := jiraEventName(r.Header, data)
	if h.debugEvents {
		logDebugEvent(logger, auth.ProviderAtlassian, eventName, rawBody)
	}

	rawObject, normalized := annotatePayload(rawObject, data, auth.ProviderAtlassian, eventName)
	tenantID, stateID, installationID, instanceKey := h.resolveStateID(r.Context(), data)
	namespaceID, namespaceName := jiraNamespaceInfo(data)

	ctx := r.Context()
	if tenantID != "" {
		ctx = storage.WithTenant(ctx, tenantID)
		r = r.WithContext(ctx)
	}

	h.emit(r, logger, core.Event{
		Provider:            auth.ProviderAtlassian,
		ProviderType:        normalized.ProviderType,
		Name:                eventName,
		EventType:           normalized.EventType,
		Action:              normalized.Action,
		ResourceType:        normalized.ResourceType,
		ResourceID:          normalized.ResourceID,
		ResourceName:        normalized.ResourceName,
		ActorID:             normalized.ActorID,
		ActorName:           normalized.ActorName,
		OccurredAt:          normalized.OccurredAt,
		RequestID:           reqID,
		Headers:             cloneHeaders(r.Header),
		Data:                data,
		RawPayload:          rawBody,
		RawObject:           rawObject,
		StateID:             stateID,
		TenantID:            tenantID,
		InstallationID:      installationID,
		ProviderInstanceKey: instanceKey,
		NamespaceID:         namespaceID,
		NamespaceName:       namespaceName,
	})

	w.WriteHeader(http.StatusOK)
}

func (h *JiraHandler) verifySignature(headers http.Header, body []byte) error {
	if h.secret == "" {
		return nil
	}
	rawSig := strings.TrimSpace(headers.Get("X-Hub-Signature"))
	if rawSig == "" {
		rawSig = strings.TrimSpace(headers.Get("X-Hub-Signature-256"))
	}
	if rawSig == "" {
		return errors.New("missing webhook signature header")
	}
	provided := strings.TrimSpace(strings.TrimPrefix(rawSig, "sha256="))
	mac := hmac.New(sha256.New, []byte(h.secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		return errors.New("signature mismatch")
	}
	return nil
}

func jiraEventName(headers http.Header, data map[string]interface{}) string {
	if headers != nil {
		if value := strings.TrimSpace(headers.Get("X-Atlassian-Webhook-Event")); value != "" {
			return value
		}
	}
	name := dataString(data, "webhookEvent", "eventType", "event_type")
	if name == "" {
		name = "event"
	}
	return name
}

func jiraNamespaceInfo(data map[string]interface{}) (string, string) {
	spaceID := dataString(data, "space.id")
	spaceKey := dataString(data, "space.key", "space.name")
	if spaceID != "" || spaceKey != "" {
		if spaceKey == "" {
			spaceKey = spaceID
		}
		return spaceID, spaceKey
	}
	projectID := dataString(data, "issue.fields.project.id", "project.id")
	projectKey := dataString(data, "issue.fields.project.key", "project.key")
	if projectKey == "" {
		projectKey = projectID
	}
	return projectID, projectKey
}

func (h *JiraHandler) resolveStateID(ctx context.Context, data map[string]interface{}) (string, string, string, string) {
	accountID := jiraAccountIDFromPayload(data)
	if accountID == "" || h.installStore == nil {
		return "", accountID, "", ""
	}
	preferredTenant := storage.TenantFromContext(ctx)
	installations, err := h.installStore.ListInstallations(ctx, auth.ProviderAtlassian, accountID)
	if err != nil || len(installations) == 0 {
		installations, err = h.installStore.ListInstallations(context.Background(), auth.ProviderAtlassian, accountID)
		if err != nil || len(installations) == 0 {
			return "", accountID, "", ""
		}
	}
	latest := pickBestJiraInstallation(installations, preferredTenant)
	return latest.TenantID, accountID, latest.InstallationID, latest.ProviderInstanceKey
}

func jiraAccountIDFromPayload(data map[string]interface{}) string {
	candidates := []string{
		dataString(data, "cloudId", "cloud_id"),
		hostFromURL(dataString(data, "issue.self", "project.self", "user.self")),
		hostFromURL(dataString(data, "serverUrl", "baseUrl", "base_url")),
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Host)
}

func pickBestJiraInstallation(records []storage.InstallRecord, preferredTenant string) storage.InstallRecord {
	preferredTenant = strings.TrimSpace(preferredTenant)
	candidates := records
	if preferredTenant != "" {
		byTenant := make([]storage.InstallRecord, 0, len(records))
		for _, record := range records {
			if strings.TrimSpace(record.TenantID) == preferredTenant {
				byTenant = append(byTenant, record)
			}
		}
		if len(byTenant) > 0 {
			candidates = byTenant
		}
	} else {
		nonEmptyTenant := make([]storage.InstallRecord, 0, len(records))
		for _, record := range records {
			if strings.TrimSpace(record.TenantID) != "" {
				nonEmptyTenant = append(nonEmptyTenant, record)
			}
		}
		if len(nonEmptyTenant) > 0 {
			candidates = nonEmptyTenant
		}
	}

	latest := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if candidates[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = candidates[i]
		}
	}
	return latest
}

func (h *JiraHandler) matchRules(ctx context.Context, event core.Event, tenantID string, logger *log.Logger) []core.MatchedRule {
	if h.ruleStore != nil {
		return matchRulesFromStore(ctx, event, tenantID, h.ruleStore, h.driverStore, h.rulesStrict, logger)
	}
	if h.rules == nil {
		return nil
	}
	return h.rules.EvaluateRulesForTenantWithLogger(event, tenantID, logger)
}

func (h *JiraHandler) emit(r *http.Request, logger *log.Logger, event core.Event) {
	if logger != nil {
		logger.Printf("event received provider=%s name=%s installation_id=%s namespace=%s request_id=%s", event.Provider, event.Name, event.InstallationID, event.NamespaceName, event.RequestID)
	}
	tenantID := storage.TenantFromContext(r.Context())
	matches := h.matchRules(r.Context(), event, tenantID, logger)
	if h.logs == nil {
		matchRules := ruleMatchesFromRules(matches)
		logger.Printf("event provider=%s name=%s topics=%v", event.Provider, event.Name, topicsFromMatches(matchRules))
		publishMatchesWithFallback(r.Context(), event, matchRules, nil, h.dynamicDrivers, h.publisher, logger, nil, nil)
		return
	}

	matchLogs := logEventMatches(r.Context(), h.logs, logger, event, matches)
	logger.Printf("event provider=%s name=%s topics=%v", event.Provider, event.Name, topicsFromLogRecords(matchLogs))
	matchRules := ruleMatchesFromRules(matches)
	statusUpdater := func(recordID, status, message string) {
		if recordID == "" {
			return
		}
		if err := h.logs.UpdateEventLogStatus(r.Context(), recordID, status, message); err != nil {
			logger.Printf("event log update failed: %v", err)
		}
	}
	payloadUpdater := func(recordID string, transformed []byte) {
		if recordID == "" {
			return
		}
		if err := h.logs.UpdateEventLogTransformedPayload(r.Context(), recordID, transformed); err != nil {
			logger.Printf("event log transformed payload update failed: %v", err)
		}
	}
	publishMatchesWithFallback(r.Context(), event, matchRules, matchLogs, h.dynamicDrivers, h.publisher, logger, statusUpdater, payloadUpdater)
}

func parseJiraJSON(body []byte) (map[string]interface{}, error) {
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid jira payload: %w", err)
	}
	return payload, nil
}
