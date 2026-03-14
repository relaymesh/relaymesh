package webhook

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-playground/webhooks/v6/github"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/drivers"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// GitHubHandler handles incoming webhooks from GitHub.
type GitHubHandler struct {
	hook           *github.Webhook
	fallbackHook   *github.Webhook
	secret         string
	providerConfig auth.ProviderConfig
	rules          *core.RuleEngine
	publisher      core.Publisher
	logger         *log.Logger
	maxBody        int64
	debugEvents    bool
	store          storage.Store
	namespaces     storage.NamespaceStore
	logs           storage.EventLogStore
	ruleStore      storage.RuleStore
	driverStore    storage.DriverStore
	rulesStrict    bool
	dynamicDrivers *drivers.DynamicPublisherCache
}

var githubEvents = []github.Event{
	github.CheckRunEvent,
	github.CheckSuiteEvent,
	github.CommitCommentEvent,
	github.CreateEvent,
	github.DeleteEvent,
	github.DependabotAlertEvent,
	github.DeployKeyEvent,
	github.DeploymentEvent,
	github.DeploymentStatusEvent,
	github.ForkEvent,
	github.GollumEvent,
	github.InstallationEvent,
	github.InstallationRepositoriesEvent,
	github.IntegrationInstallationEvent,
	github.IntegrationInstallationRepositoriesEvent,
	github.IssueCommentEvent,
	github.IssuesEvent,
	github.LabelEvent,
	github.MemberEvent,
	github.MembershipEvent,
	github.MilestoneEvent,
	github.MetaEvent,
	github.OrganizationEvent,
	github.OrgBlockEvent,
	github.PageBuildEvent,
	github.PingEvent,
	github.ProjectCardEvent,
	github.ProjectColumnEvent,
	github.ProjectEvent,
	github.PublicEvent,
	github.PullRequestEvent,
	github.PullRequestReviewEvent,
	github.PullRequestReviewCommentEvent,
	github.PushEvent,
	github.ReleaseEvent,
	github.RepositoryEvent,
	github.RepositoryVulnerabilityAlertEvent,
	github.SecurityAdvisoryEvent,
	github.StatusEvent,
	github.TeamEvent,
	github.TeamAddEvent,
	github.WatchEvent,
	github.WorkflowDispatchEvent,
	github.WorkflowJobEvent,
	github.WorkflowRunEvent,
	github.GitHubAppAuthorizationEvent,
}

// NewGitHubHandler creates a new GitHubHandler.
func NewGitHubHandler(secret string, rules *core.RuleEngine, publisher core.Publisher, logger *log.Logger, maxBody int64, debugEvents bool, store storage.Store, namespaces storage.NamespaceStore, logs storage.EventLogStore, ruleStore storage.RuleStore, driverStore storage.DriverStore, rulesStrict bool, dynamicDrivers *drivers.DynamicPublisherCache, cfg auth.ProviderConfig) (*GitHubHandler, error) {
	hook, err := github.New(github.Options.Secret(secret))
	if err != nil {
		return nil, err
	}
	fallbackHook, err := github.New()
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = log.Default()
	}
	return &GitHubHandler{
		hook:           hook,
		fallbackHook:   fallbackHook,
		secret:         secret,
		providerConfig: cfg,
		rules:          rules,
		publisher:      publisher,
		logger:         logger,
		maxBody:        maxBody,
		debugEvents:    debugEvents,
		store:          store,
		namespaces:     namespaces,
		logs:           logs,
		ruleStore:      ruleStore,
		driverStore:    driverStore,
		rulesStrict:    rulesStrict,
		dynamicDrivers: dynamicDrivers,
	}, nil
}

func (h *GitHubHandler) githubWebBaseURL() string {
	if h == nil {
		return "https://github.com"
	}
	if base := strings.TrimRight(h.providerConfig.API.WebBaseURL, "/"); base != "" {
		return base
	}
	apiBase := strings.TrimSpace(h.providerConfig.API.BaseURL)
	if apiBase == "" || apiBase == "https://api.github.com" {
		return "https://github.com"
	}
	trimmed := strings.TrimSuffix(apiBase, "/api/v3")
	trimmed = strings.TrimSuffix(trimmed, "/api")
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return "https://github.com"
	}
	return trimmed
}

// ServeHTTP handles an incoming HTTP request.
func (h *GitHubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r, logger, reqID, rawBody, ok := prepareWebhookRequest(w, r, h.maxBody, h.logger)
	if !ok {
		return
	}

	if h.debugEvents {
		logDebugEvent(logger, "github", r.Header.Get("X-GitHub-Event"), rawBody)
	}

	payload, err := h.hook.Parse(r, githubEvents...)
	if err != nil {
		if errors.Is(err, github.ErrEventNotFound) {
			if h.secret != "" {
				if !verifyGitHubSignature(h.secret, rawBody, r.Header.Get("X-Hub-Signature-256")) &&
					!verifyGitHubSignature(h.secret, rawBody, r.Header.Get("X-Hub-Signature")) {
					logger.Printf("github webhook signature invalid for unknown event %s", r.Header.Get("X-GitHub-Event"))
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
			}
			logger.Printf("github webhook accepted unknown event %s", r.Header.Get("X-GitHub-Event"))
			payload = nil
		} else {
			if errors.Is(err, github.ErrMissingHubSignatureHeader) && h.secret != "" {
				sha1Header := r.Header.Get("X-Hub-Signature")
				if sha1Header != "" && verifyGitHubSHA1(h.secret, rawBody, sha1Header) {
					logger.Printf("github parse warning: %v; accepted sha1 signature", err)
					r.Body = io.NopCloser(bytes.NewReader(rawBody))
					payload, err = h.fallbackHook.Parse(r, githubEvents...)
				}
			}
			if err != nil {
				logger.Printf("github parse failed: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}

	eventName := r.Header.Get("X-GitHub-Event")
	switch payload.(type) {
	case github.PingPayload:
		w.WriteHeader(http.StatusOK)
		return
	default:
		rawObject, data := rawObjectAndFlatten(rawBody)
		rawObject = annotatePayload(rawObject, data, "github", eventName)
		namespaceID, namespaceName := githubNamespaceInfo(rawBody)
		tenantID, stateID, installationID, instanceKey := h.resolveStateID(r.Context(), rawBody)
		if installationID == "" {
			logger.Printf("github webhook ignored: missing installation_id")
			w.WriteHeader(http.StatusOK)
			return
		}
		ctx := r.Context()
		if tenantID != "" {
			ctx = storage.WithTenant(ctx, tenantID)
		}
		r = r.WithContext(ctx)
		if err := h.applyInstallSystemRules(ctx, eventName, rawBody); err != nil {
			logger.Printf("github install sync failed: %v", err)
		}
		h.emit(r, logger, core.Event{
			Provider:            "github",
			Name:                eventName,
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

func (h *GitHubHandler) emit(r *http.Request, logger *log.Logger, event core.Event) {
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
