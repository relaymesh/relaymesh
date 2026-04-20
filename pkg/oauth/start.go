package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/providerinstance"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// StartHandler redirects users into provider install/authorize flows.
type StartHandler struct {
	Providers             auth.Config
	Endpoint              string
	Logger                *log.Logger
	ProviderInstanceStore storage.ProviderInstanceStore
	ProviderInstanceCache *providerinstance.Cache
	Registry              *Registry
}

func (h *StartHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	logger := h.Logger
	if logger == nil {
		logger = log.Default()
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	provider := auth.NormalizeProviderName(r.URL.Query().Get("provider"))
	if provider == "" {
		http.Error(w, "missing provider", http.StatusBadRequest)
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		state = randomState()
	}
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	instanceKey := strings.TrimSpace(r.URL.Query().Get("instance"))
	logger.Printf("oauth start request provider=%s tenant=%s instance=%s state=%s", provider, tenantID, instanceKey, state)
	ctx := storage.WithTenant(r.Context(), tenantID)
	providerCfg, resolvedKey := h.resolveProviderConfig(ctx, provider, instanceKey)
	if resolvedKey != "" {
		instanceKey = resolvedKey
	}
	logger.Printf("oauth start resolved instance=%s app_slug=%q oauth_client_id=%q",
		instanceKey, providerCfg.App.AppSlug, providerCfg.OAuth.ClientID)
	state = encodeState(state, tenantID, instanceKey)

	registry := h.Registry
	if registry == nil {
		registry = DefaultRegistry()
	}
	plugin, ok := registry.Provider(provider)
	if !ok {
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}
	target, err := plugin.AuthorizeURL(r, providerCfg, state, h.Endpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *StartHandler) resolveProviderConfig(ctx context.Context, provider, instanceKey string) (auth.ProviderConfig, string) {
	fallback, _ := h.Providers.ProviderConfigFor(provider)
	instanceKey = strings.TrimSpace(instanceKey)

	if instanceKey != "" {
		if cfg, ok := h.lookupProviderInstanceConfig(ctx, provider, instanceKey); ok {
			return cfg, instanceKey
		}
		if cfg, ok := h.lookupProviderInstanceConfig(context.Background(), provider, instanceKey); ok {
			return cfg, instanceKey
		}
		return fallback, instanceKey
	}

	if h.ProviderInstanceStore != nil {
		records, err := h.ProviderInstanceStore.ListProviderInstances(ctx, provider)
		if err == nil && len(records) == 1 {
			cfg, err := providerinstance.ProviderConfigFromRecord(records[0])
			if err == nil {
				return cfg, records[0].Key
			}
		}
	}

	return fallback, ""
}

func (h *StartHandler) lookupProviderInstanceConfig(ctx context.Context, provider, instanceKey string) (auth.ProviderConfig, bool) {
	if h.ProviderInstanceCache != nil {
		if cfg, ok, err := h.ProviderInstanceCache.ConfigFor(ctx, provider, instanceKey); err == nil && ok {
			return cfg, true
		}
	}
	if h.ProviderInstanceStore != nil {
		record, err := h.ProviderInstanceStore.GetProviderInstance(ctx, provider, instanceKey)
		if err == nil && record != nil {
			cfg, err := providerinstance.ProviderConfigFromRecord(*record)
			if err == nil {
				return cfg, true
			}
		}
	}
	return auth.ProviderConfig{}, false
}

func githubInstallURL(cfg auth.ProviderConfig, state string) (string, error) {
	appSlug := strings.TrimSpace(cfg.App.AppSlug)
	if appSlug == "" {
		return "", fmt.Errorf("github app_slug is required")
	}
	webBase := githubWebBase(cfg)
	return addQueryParam(fmt.Sprintf("%s/apps/%s/installations/new", webBase, appSlug), "state", state)
}

func gitlabAuthorizeURL(cfg auth.ProviderConfig, state, redirectURL string) (string, error) {
	if cfg.OAuth.ClientID == "" {
		return "", fmt.Errorf("gitlab oauth_client_id is required")
	}
	webBase := gitlabWebBase(cfg)
	u, err := url.Parse(webBase + "/oauth/authorize")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", cfg.OAuth.ClientID)
	q.Set("response_type", "code")
	if redirectURL != "" {
		q.Set("redirect_uri", redirectURL)
	}
	if len(cfg.OAuth.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.OAuth.Scopes, " "))
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func bitbucketAuthorizeURL(cfg auth.ProviderConfig, state, redirectURL string) (string, error) {
	if cfg.OAuth.ClientID == "" {
		return "", fmt.Errorf("bitbucket oauth_client_id is required")
	}
	webBase := strings.TrimRight(cfg.API.WebBaseURL, "/")
	if webBase == "" {
		webBase = "https://bitbucket.org"
	}
	u, err := url.Parse(webBase + "/site/oauth2/authorize")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", cfg.OAuth.ClientID)
	q.Set("response_type", "code")
	if redirectURL != "" {
		q.Set("redirect_uri", redirectURL)
	}
	if len(cfg.OAuth.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.OAuth.Scopes, " "))
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func slackAuthorizeURL(cfg auth.ProviderConfig, state, redirectURL string) (string, error) {
	if cfg.OAuth.ClientID == "" {
		return "", fmt.Errorf("slack oauth_client_id is required")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://slack.com/api"
	}
	authorizeBase := strings.TrimSuffix(baseURL, "/api")
	u, err := url.Parse(authorizeBase + "/oauth/v2/authorize")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", cfg.OAuth.ClientID)
	if redirectURL != "" {
		q.Set("redirect_uri", redirectURL)
	}
	if len(cfg.OAuth.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.OAuth.Scopes, ","))
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func githubWebBase(cfg auth.ProviderConfig) string {
	webBase := strings.TrimRight(cfg.API.WebBaseURL, "/")
	if webBase != "" {
		return webBase
	}
	base := strings.TrimRight(cfg.API.BaseURL, "/")
	if base == "" || base == "https://api.github.com" {
		return "https://github.com"
	}
	webBase = strings.TrimSuffix(base, "/api/v3")
	webBase = strings.TrimSuffix(webBase, "/api")
	if webBase == "" {
		return "https://github.com"
	}
	return webBase
}

func gitlabWebBase(cfg auth.ProviderConfig) string {
	webBase := strings.TrimRight(cfg.API.WebBaseURL, "/")
	if webBase != "" {
		return webBase
	}
	base := strings.TrimRight(cfg.API.BaseURL, "/")
	if base == "" {
		return "https://gitlab.com"
	}
	webBase = strings.TrimSuffix(base, "/api/v4")
	if webBase == "" {
		return "https://gitlab.com"
	}
	return webBase
}

func addQueryParam(rawURL, key, value string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if value == "" {
		return u.String(), nil
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func randomState() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}
