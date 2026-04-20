package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"

	"github.com/go-playground/webhooks/v6/bitbucket"
)

// BitbucketHandler handles incoming webhooks from Bitbucket.
type BitbucketHandler struct {
	hook           *bitbucket.Webhook
	rules          *core.RuleEngine
	publisher      core.Publisher
	logger         *log.Logger
	maxBody        int64
	debugEvents    bool
	namespaces     storage.NamespaceStore
	logs           storage.EventLogStore
	ruleStore      storage.RuleStore
	driverStore    storage.DriverStore
	rulesStrict    bool
	dynamicDrivers *drivers.DynamicPublisherCache
}

var bitbucketEvents = []bitbucket.Event{
	bitbucket.RepoPushEvent,
	bitbucket.RepoForkEvent,
	bitbucket.RepoUpdatedEvent,
	bitbucket.RepoCommitCommentCreatedEvent,
	bitbucket.RepoCommitStatusCreatedEvent,
	bitbucket.RepoCommitStatusUpdatedEvent,
	bitbucket.IssueCreatedEvent,
	bitbucket.IssueUpdatedEvent,
	bitbucket.IssueCommentCreatedEvent,
	bitbucket.PullRequestCreatedEvent,
	bitbucket.PullRequestUpdatedEvent,
	bitbucket.PullRequestApprovedEvent,
	bitbucket.PullRequestUnapprovedEvent,
	bitbucket.PullRequestMergedEvent,
	bitbucket.PullRequestDeclinedEvent,
	bitbucket.PullRequestCommentCreatedEvent,
	bitbucket.PullRequestCommentUpdatedEvent,
	bitbucket.PullRequestCommentDeletedEvent,
}

// NewBitbucketHandler creates a new BitbucketHandler.
func NewBitbucketHandler(secret string, rules *core.RuleEngine, publisher core.Publisher, logger *log.Logger, maxBody int64, debugEvents bool, namespaces storage.NamespaceStore, logs storage.EventLogStore, ruleStore storage.RuleStore, driverStore storage.DriverStore, rulesStrict bool, dynamicDrivers *drivers.DynamicPublisherCache) (*BitbucketHandler, error) {
	options := make([]bitbucket.Option, 0, 1)
	if secret != "" {
		options = append(options, bitbucket.Options.UUID(secret))
	}
	hook, err := bitbucket.New(options...)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.Default()
	}
	return &BitbucketHandler{
		hook:           hook,
		rules:          rules,
		publisher:      publisher,
		logger:         logger,
		maxBody:        maxBody,
		debugEvents:    debugEvents,
		namespaces:     namespaces,
		logs:           logs,
		ruleStore:      ruleStore,
		driverStore:    driverStore,
		rulesStrict:    rulesStrict,
		dynamicDrivers: dynamicDrivers,
	}, nil
}

// ServeHTTP handles an incoming HTTP request.
func (h *BitbucketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r, logger, reqID, rawBody, ok := prepareWebhookRequest(w, r, h.maxBody, h.logger)
	if !ok {
		return
	}

	eventName := r.Header.Get("X-Event-Key")
	if h.debugEvents {
		logDebugEvent(logger, "bitbucket", eventName, rawBody)
	}

	payload, err := h.hook.Parse(r, bitbucketEvents...)
	if err != nil {
		if errors.Is(err, bitbucket.ErrEventNotFound) {
			logger.Printf("bitbucket webhook accepted unknown event %s", eventName)
			payload = nil
		} else {
			if errors.Is(err, bitbucket.ErrMissingHookUUIDHeader) {
				logger.Printf("bitbucket parse warning: %v; skipping UUID verification", err)
				r.Body = io.NopCloser(bytes.NewReader(rawBody))
				unverified, fallbackErr := bitbucket.New()
				if fallbackErr == nil {
					payload, err = unverified.Parse(r, bitbucketEvents...)
				} else {
					err = fallbackErr
				}
			}
			if err != nil {
				logger.Printf("bitbucket parse failed: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}

	switch payload.(type) {
	default:
		rawObject, data := rawObjectAndFlatten(rawBody)
		rawObject, normalized := annotatePayload(rawObject, data, "bitbucket", eventName)
		namespaceID, namespaceName := bitbucketNamespaceInfo(rawBody)
		tenantID, stateID, installationID, instanceKey := h.resolveStateID(r.Context(), rawBody)
		ctx := r.Context()
		if tenantID != "" {
			ctx = storage.WithTenant(ctx, tenantID)
			r = r.WithContext(ctx)
		}
		if installationID == "" {
			logger.Printf("bitbucket webhook ignored: missing installation_id")
			w.WriteHeader(http.StatusOK)
			return
		}
		h.emit(r, logger, core.Event{
			Provider:            "bitbucket",
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
			NamespaceID:         namespaceID,
			NamespaceName:       namespaceName,
		})
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BitbucketHandler) resolveStateID(ctx context.Context, raw []byte) (string, string, string, string) {
	if h.namespaces == nil {
		return "", "", "", ""
	}
	var payload struct {
		Repository struct {
			UUID string `json:"uuid"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", ""
	}
	repoID := strings.TrimSpace(payload.Repository.UUID)
	if repoID == "" {
		return "", "", "", ""
	}
	record, err := h.namespaces.GetNamespace(ctx, "bitbucket", repoID, "")
	if err != nil || record == nil {
		return "", "", "", ""
	}
	return record.TenantID, record.AccountID, record.InstallationID, record.ProviderInstanceKey
}

func (h *BitbucketHandler) matchRules(ctx context.Context, event core.Event, tenantID string, logger *log.Logger) []core.MatchedRule {
	if h.ruleStore != nil {
		return matchRulesFromStore(ctx, event, tenantID, h.ruleStore, h.driverStore, h.rulesStrict, logger)
	}
	if h.rules == nil {
		return nil
	}
	return h.rules.EvaluateRulesForTenantWithLogger(event, tenantID, logger)
}

func (h *BitbucketHandler) emit(r *http.Request, logger *log.Logger, event core.Event) {
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

func bitbucketNamespaceInfo(raw []byte) (string, string) {
	var payload struct {
		Repository struct {
			UUID     string `json:"uuid"`
			FullName string `json:"full_name"`
			Name     string `json:"name"`
			Owner    struct {
				Username string `json:"username"`
				Nickname string `json:"nickname"`
			} `json:"owner"`
			Workspace struct {
				Slug string `json:"slug"`
			} `json:"workspace"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	namespaceID := strings.TrimSpace(payload.Repository.UUID)
	namespaceName := strings.TrimSpace(payload.Repository.FullName)
	if namespaceName == "" && payload.Repository.Workspace.Slug != "" && payload.Repository.Name != "" {
		namespaceName = payload.Repository.Workspace.Slug + "/" + payload.Repository.Name
	}
	if namespaceName == "" && payload.Repository.Owner.Username != "" && payload.Repository.Name != "" {
		namespaceName = payload.Repository.Owner.Username + "/" + payload.Repository.Name
	}
	if namespaceName == "" && payload.Repository.Owner.Nickname != "" && payload.Repository.Name != "" {
		namespaceName = payload.Repository.Owner.Nickname + "/" + payload.Repository.Name
	}
	return namespaceID, namespaceName
}
