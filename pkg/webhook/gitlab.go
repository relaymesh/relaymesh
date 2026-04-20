package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"

	"github.com/go-playground/webhooks/v6/gitlab"
)

// GitLabHandler handles incoming webhooks from GitLab.
type GitLabHandler struct {
	hook           *gitlab.Webhook
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

var gitlabEvents = []gitlab.Event{
	gitlab.PushEvents,
	gitlab.TagEvents,
	gitlab.IssuesEvents,
	gitlab.ConfidentialIssuesEvents,
	gitlab.CommentEvents,
	gitlab.ConfidentialCommentEvents,
	gitlab.MergeRequestEvents,
	gitlab.WikiPageEvents,
	gitlab.PipelineEvents,
	gitlab.BuildEvents,
	gitlab.JobEvents,
	gitlab.DeploymentEvents,
	gitlab.SystemHookEvents,
}

// NewGitLabHandler creates a new GitLabHandler.
func NewGitLabHandler(secret string, rules *core.RuleEngine, publisher core.Publisher, logger *log.Logger, maxBody int64, debugEvents bool, namespaces storage.NamespaceStore, logs storage.EventLogStore, ruleStore storage.RuleStore, driverStore storage.DriverStore, rulesStrict bool, dynamicDrivers *drivers.DynamicPublisherCache) (*GitLabHandler, error) {
	options := make([]gitlab.Option, 0, 1)
	if secret != "" {
		options = append(options, gitlab.Options.Secret(secret))
	}
	hook, err := gitlab.New(options...)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.Default()
	}
	return &GitLabHandler{
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
func (h *GitLabHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r, logger, reqID, rawBody, ok := prepareWebhookRequest(w, r, h.maxBody, h.logger)
	if !ok {
		return
	}

	eventName := r.Header.Get("X-Gitlab-Event")
	if h.debugEvents {
		logDebugEvent(logger, "gitlab", eventName, rawBody)
	}

	payload, err := h.hook.Parse(r, gitlabEvents...)
	if err != nil {
		if errors.Is(err, gitlab.ErrEventNotFound) {
			logger.Printf("gitlab webhook accepted unknown event %s", eventName)
			payload = nil
		} else {
			logger.Printf("gitlab parse failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	switch payload.(type) {
	default:
		rawObject, data := rawObjectAndFlatten(rawBody)
		rawObject, normalized := annotatePayload(rawObject, data, "gitlab", eventName)
		namespaceID, namespaceName := gitlabNamespaceInfo(rawBody)
		tenantID, stateID, installationID, instanceKey := h.resolveStateID(r.Context(), rawBody)
		if installationID == "" {
			logger.Printf("gitlab webhook ignored: missing installation_id")
			w.WriteHeader(http.StatusOK)
			return
		}
		ctx := r.Context()
		if tenantID != "" {
			ctx = storage.WithTenant(ctx, tenantID)
			r = r.WithContext(ctx)
		}
		h.emit(r, logger, core.Event{
			Provider:            "gitlab",
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

func (h *GitLabHandler) resolveStateID(ctx context.Context, raw []byte) (string, string, string, string) {
	if h.namespaces == nil {
		return "", "", "", ""
	}
	var payload struct {
		Project struct {
			ID int64 `json:"id"`
		} `json:"project"`
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", ""
	}
	repoID := payload.Project.ID
	if repoID == 0 {
		repoID = payload.ProjectID
	}
	if repoID == 0 {
		return "", "", "", ""
	}
	record, err := h.namespaces.GetNamespace(ctx, "gitlab", strconv.FormatInt(repoID, 10), "")
	if err != nil || record == nil {
		return "", "", "", ""
	}
	return record.TenantID, record.AccountID, record.InstallationID, record.ProviderInstanceKey
}

func (h *GitLabHandler) emit(r *http.Request, logger *log.Logger, event core.Event) {
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

func (h *GitLabHandler) matchRules(ctx context.Context, event core.Event, tenantID string, logger *log.Logger) []core.MatchedRule {
	if h.ruleStore != nil {
		return matchRulesFromStore(ctx, event, tenantID, h.ruleStore, h.driverStore, h.rulesStrict, logger)
	}
	if h.rules == nil {
		return nil
	}
	return h.rules.EvaluateRulesForTenantWithLogger(event, tenantID, logger)
}

func gitlabNamespaceInfo(raw []byte) (string, string) {
	var payload struct {
		Project struct {
			ID                int64  `json:"id"`
			PathWithNamespace string `json:"path_with_namespace"`
			Path              string `json:"path"`
			Namespace         string `json:"namespace"`
		} `json:"project"`
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	repoID := payload.Project.ID
	if repoID == 0 {
		repoID = payload.ProjectID
	}
	namespaceID := ""
	if repoID > 0 {
		namespaceID = strconv.FormatInt(repoID, 10)
	}
	namespaceName := strings.TrimSpace(payload.Project.PathWithNamespace)
	if namespaceName == "" && payload.Project.Namespace != "" && payload.Project.Path != "" {
		namespaceName = payload.Project.Namespace + "/" + payload.Project.Path
	}
	return namespaceID, namespaceName
}
