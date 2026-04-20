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
	"strconv"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

const slackRequestMaxAge = 5 * time.Minute

type SlackHandler struct {
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

func NewSlackHandler(secret string, rules *core.RuleEngine, publisher core.Publisher, logger *log.Logger, maxBody int64, debugEvents bool, installStore storage.Store, logs storage.EventLogStore, ruleStore storage.RuleStore, driverStore storage.DriverStore, rulesStrict bool, dynamicDrivers *drivers.DynamicPublisherCache) (*SlackHandler, error) {
	if logger == nil {
		logger = log.Default()
	}
	return &SlackHandler{
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

func (h *SlackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r, logger, reqID, rawBody, ok := prepareWebhookRequest(w, r, h.maxBody, h.logger)
	if !ok {
		return
	}

	if err := h.verifySlackSignature(r.Header, rawBody); err != nil {
		logger.Printf("slack signature verification failed: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	rawObject, data := rawObjectAndFlatten(rawBody)
	if rawObject == nil {
		logger.Printf("slack payload invalid json")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventName := slackEventName(r.Header, data)
	if h.debugEvents {
		logDebugEvent(logger, "slack", eventName, rawBody)
	}

	if strings.EqualFold(dataString(data, "type"), "url_verification") {
		challenge := dataString(data, "challenge")
		if challenge == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(challenge))
		return
	}

	rawObject, normalized := annotatePayload(rawObject, data, auth.ProviderSlack, eventName)
	tenantID, stateID, installationID, instanceKey := h.resolveStateID(r.Context(), data)
	ctx := r.Context()
	if tenantID != "" {
		ctx = storage.WithTenant(ctx, tenantID)
		r = r.WithContext(ctx)
	}

	h.emit(r, logger, core.Event{
		Provider:            auth.ProviderSlack,
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
		TenantID:            storage.TenantFromContext(r.Context()),
		InstallationID:      installationID,
		ProviderInstanceKey: instanceKey,
	})

	w.WriteHeader(http.StatusOK)
}

func (h *SlackHandler) verifySlackSignature(headers http.Header, body []byte) error {
	if h.secret == "" {
		return nil
	}
	timestamp := strings.TrimSpace(headers.Get("X-Slack-Request-Timestamp"))
	if timestamp == "" {
		return errors.New("missing X-Slack-Request-Timestamp")
	}
	signature := strings.TrimSpace(headers.Get("X-Slack-Signature"))
	if signature == "" {
		return errors.New("missing X-Slack-Signature")
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errors.New("invalid X-Slack-Request-Timestamp")
	}
	if age := time.Since(time.Unix(ts, 0)); age > slackRequestMaxAge || age < -slackRequestMaxAge {
		return errors.New("stale slack request timestamp")
	}
	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(h.secret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return errors.New("signature mismatch")
	}
	return nil
}

func slackEventName(headers http.Header, data map[string]interface{}) string {
	if headers != nil {
		if value := strings.TrimSpace(headers.Get("X-Slack-Event-Type")); value != "" {
			return value
		}
	}
	typ := dataString(data, "type")
	if strings.EqualFold(typ, "event_callback") {
		inner := dataString(data, "event.type")
		if inner != "" {
			return "event_callback." + inner
		}
	}
	if typ != "" {
		return typ
	}
	return "event"
}

func (h *SlackHandler) resolveStateID(ctx context.Context, data map[string]interface{}) (string, string, string, string) {
	teamID := dataString(data, "team_id", "team.id", "authorizations.0.team_id")
	if teamID == "" || h.installStore == nil {
		return "", teamID, "", ""
	}
	installations, err := h.installStore.ListInstallations(ctx, auth.ProviderSlack, teamID)
	if err != nil || len(installations) == 0 {
		installations, err = h.installStore.ListInstallations(context.Background(), auth.ProviderSlack, teamID)
		if err != nil || len(installations) == 0 {
			return "", teamID, "", ""
		}
	}
	latest := pickLatestInstallation(installations)
	return latest.TenantID, teamID, latest.InstallationID, latest.ProviderInstanceKey
}

func pickLatestInstallation(records []storage.InstallRecord) storage.InstallRecord {
	latest := records[0]
	for i := 1; i < len(records); i++ {
		if records[i].UpdatedAt.After(latest.UpdatedAt) {
			latest = records[i]
		}
	}
	return latest
}

func (h *SlackHandler) matchRules(ctx context.Context, event core.Event, tenantID string, logger *log.Logger) []core.MatchedRule {
	if h.ruleStore != nil {
		return matchRulesFromStore(ctx, event, tenantID, h.ruleStore, h.driverStore, h.rulesStrict, logger)
	}
	if h.rules == nil {
		return nil
	}
	return h.rules.EvaluateRulesForTenantWithLogger(event, tenantID, logger)
}

func (h *SlackHandler) emit(r *http.Request, logger *log.Logger, event core.Event) {
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

func computeSlackSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":" + string(body)))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func parseSlackJSON(body []byte) (map[string]interface{}, error) {
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid slack payload: %w", err)
	}
	return payload, nil
}
