package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func (h *Handler) handleJira(w http.ResponseWriter, r *http.Request, logger *log.Logger, cfg auth.ProviderConfig) {
	stateValue := decodeState(r.URL.Query().Get("state"))
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	storeCtx := storage.WithTenant(r.Context(), stateValue.TenantID)
	cfg, instanceKey, instanceRedirect := h.resolveInstanceConfig(storeCtx, auth.ProviderAtlassian, stateValue.InstanceKey, cfg)

	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		http.Error(w, "oauth client config missing", http.StatusInternalServerError)
		return
	}

	redirectURL := callbackURL(r, auth.ProviderAtlassian, h.Endpoint)
	token, resource, metadataJSON, err := exchangeJiraToken(r.Context(), cfg, code, redirectURL)
	if err != nil {
		logger.Printf("jira oauth exchange failed: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	accountID := strings.TrimSpace(resource.SiteHost)
	if accountID == "" {
		accountID = strings.TrimSpace(resource.CloudID)
	}
	if accountID == "" {
		http.Error(w, "jira account resolution failed", http.StatusBadRequest)
		return
	}
	installationID := strings.TrimSpace(resource.CloudID)
	if installationID == "" {
		installationID = accountID
	}
	webhookMeta, webhookWarning := h.autoRegisterAtlassianWebhooks(r.Context(), r, cfg, token.AccessToken, resource)
	if len(webhookMeta) > 0 {
		metadataJSON = mergeMetadataJSON(metadataJSON, webhookMeta)
	}

	warning := ""
	if webhookWarning != "" {
		warning = webhookWarning
	}
	record := storage.InstallRecord{
		TenantID:            stateValue.TenantID,
		Provider:            auth.ProviderAtlassian,
		AccountID:           accountID,
		AccountName:         resource.Name,
		InstallationID:      installationID,
		ProviderInstanceKey: instanceKey,
		AccessToken:         token.AccessToken,
		RefreshToken:        token.RefreshToken,
		ExpiresAt:           token.ExpiresAt,
		MetadataJSON:        metadataJSON,
	}

	if storeAvailable(h.Store) {
		logUpsertAttempt(logger, record, token.AccessToken)
		if err := h.Store.UpsertInstallation(storeCtx, record); err != nil {
			logger.Printf("jira installation upsert failed: %v", err)
			if warning == "" {
				warning = "install record not saved"
			} else {
				warning += "; install record not saved"
			}
		}
		dedupeInstallations(storeCtx, h.Store, auth.ProviderAtlassian, accountID, instanceKey, record.InstallationID)
	}

	params := map[string]string{
		"provider":        auth.ProviderAtlassian,
		"account_id":      accountID,
		"account_name":    resource.Name,
		"installation_id": installationID,
		"warning":         warning,
	}
	h.redirectOrJSON(w, r, params, instanceRedirect)
}

func (h *Handler) autoRegisterAtlassianWebhooks(ctx context.Context, r *http.Request, cfg auth.ProviderConfig, accessToken string, resource jiraAccessibleResource) (map[string]interface{}, string) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, ""
	}
	webhookURL := atlassianWebhookURL(r, h.Endpoint, cfg.Webhook.Path)
	if webhookURL == "" {
		return nil, "webhook auto-register skipped (public webhook url missing)"
	}
	meta := map[string]interface{}{}
	warnings := make([]string, 0, 2)

	jiraIDs, jiraErr := registerJiraWebhooks(ctx, accessToken, resource.URL, webhookURL, strings.TrimSpace(cfg.Webhook.Secret))
	if jiraErr != nil {
		warnings = append(warnings, "jira webhook registration failed")
	} else if len(jiraIDs) > 0 {
		meta["jira_webhook_ids"] = jiraIDs
	}

	confluenceIDs, confErr := registerConfluenceWebhooks(ctx, accessToken, resource.URL, webhookURL, strings.TrimSpace(cfg.Webhook.Secret))
	if confErr != nil {
		warnings = append(warnings, "confluence webhook registration failed")
	} else if len(confluenceIDs) > 0 {
		meta["confluence_webhook_ids"] = confluenceIDs
	}

	if len(meta) > 0 {
		meta["webhook_url"] = webhookURL
	}
	return meta, strings.Join(warnings, "; ")
}

func atlassianWebhookURL(r *http.Request, endpoint, path string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		scheme := forwardedProto(r)
		host := forwardedHost(r)
		if scheme == "" {
			scheme = "http"
		}
		if host == "" {
			host = r.Host
		}
		if host == "" {
			return ""
		}
		base = scheme + "://" + host
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/webhooks/atlassian"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func registerJiraWebhooks(ctx context.Context, accessToken, siteURL, webhookURL, secret string) ([]string, error) {
	siteURL = strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if siteURL == "" {
		return nil, errors.New("jira site url missing")
	}
	payload := map[string]interface{}{
		"name":   "relaymesh-atlassian",
		"url":    webhookURL,
		"events": []string{"jira:issue_created", "jira:issue_updated", "comment_created", "comment_updated"},
	}
	if secret != "" {
		payload["secret"] = secret
	}
	out, err := atlassianJSONPost(ctx, accessToken, siteURL+"/rest/webhooks/1.0/webhook", payload)
	if err != nil {
		return nil, err
	}
	return extractWebhookIDs(out), nil
}

func registerConfluenceWebhooks(ctx context.Context, accessToken, siteURL, webhookURL, secret string) ([]string, error) {
	siteURL = strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if siteURL == "" {
		return nil, errors.New("confluence site url missing")
	}
	payload := map[string]interface{}{
		"name":   "relaymesh-atlassian",
		"url":    webhookURL,
		"events": []string{"page_created", "page_updated", "page_removed"},
	}
	if secret != "" {
		payload["secret"] = secret
	}
	out, err := atlassianJSONPost(ctx, accessToken, siteURL+"/wiki/rest/webhooks/1.0/webhook", payload)
	if err != nil {
		return nil, err
	}
	return extractWebhookIDs(out), nil
}

func atlassianJSONPost(ctx context.Context, accessToken, endpoint string, payload map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("atlassian webhook register close failed: %v", err)
		}
	}()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("atlassian webhook register failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	out := map[string]interface{}{}
	if len(strings.TrimSpace(string(body))) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func extractWebhookIDs(payload map[string]interface{}) []string {
	if payload == nil {
		return nil
	}
	ids := make([]string, 0)
	if value, ok := payload["id"]; ok && value != nil {
		ids = append(ids, strings.TrimSpace(fmt.Sprintf("%v", value)))
	}
	if values, ok := payload["ids"].([]interface{}); ok {
		for _, value := range values {
			trimmed := strings.TrimSpace(fmt.Sprintf("%v", value))
			if trimmed != "" {
				ids = append(ids, trimmed)
			}
		}
	}
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			clean = append(clean, id)
		}
	}
	return clean
}

func mergeMetadataJSON(base string, extra map[string]interface{}) string {
	merged := map[string]interface{}{}
	if strings.TrimSpace(base) != "" {
		_ = json.Unmarshal([]byte(base), &merged)
	}
	for key, value := range extra {
		merged[key] = value
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		return base
	}
	return string(raw)
}

type jiraAccessibleResource struct {
	CloudID  string
	Name     string
	URL      string
	SiteHost string
	Scopes   []string
}

func exchangeJiraToken(ctx context.Context, cfg auth.ProviderConfig, code, redirectURL string) (oauthToken, jiraAccessibleResource, string, error) {
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://auth.atlassian.com"
	}
	endpoint := baseURL + "/oauth/token"

	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     cfg.OAuth.ClientID,
		"client_secret": cfg.OAuth.ClientSecret,
		"code":          code,
	}
	if redirectURL != "" {
		payload["redirect_uri"] = redirectURL
	}
	raw, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return oauthToken{}, jiraAccessibleResource{}, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthToken{}, jiraAccessibleResource{}, "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("jira token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return oauthToken{}, jiraAccessibleResource{}, "", fmt.Errorf("jira token exchange failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oauthToken{}, jiraAccessibleResource{}, "", err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return oauthToken{}, jiraAccessibleResource{}, "", errors.New("jira access token missing")
	}

	resource, err := fetchJiraAccessibleResource(ctx, token.AccessToken)
	if err != nil {
		return oauthToken{}, jiraAccessibleResource{}, "", err
	}
	metadata := map[string]interface{}{
		"token_type": token.TokenType,
		"scope":      token.Scope,
		"cloud_id":   resource.CloudID,
		"site_url":   resource.URL,
		"site_host":  resource.SiteHost,
		"scopes":     resource.Scopes,
	}
	metaRaw, _ := json.Marshal(metadata)
	return token, resource, string(metaRaw), nil
}

func fetchJiraAccessibleResource(ctx context.Context, accessToken string) (jiraAccessibleResource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return jiraAccessibleResource{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return jiraAccessibleResource{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("jira accessible-resources close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return jiraAccessibleResource{}, fmt.Errorf("jira accessible-resources failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var resources []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return jiraAccessibleResource{}, err
	}
	if len(resources) == 0 {
		return jiraAccessibleResource{}, errors.New("jira accessible resources empty")
	}
	picked := resources[0]
	siteHost := ""
	if parsed, err := url.Parse(strings.TrimSpace(picked.URL)); err == nil {
		siteHost = strings.TrimSpace(parsed.Host)
	}
	return jiraAccessibleResource{
		CloudID:  strings.TrimSpace(picked.ID),
		Name:     strings.TrimSpace(picked.Name),
		URL:      strings.TrimSpace(picked.URL),
		SiteHost: siteHost,
		Scopes:   append([]string(nil), picked.Scopes...),
	}, nil
}
