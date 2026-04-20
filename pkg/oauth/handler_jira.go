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

	warning := ""
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
			warning = "install record not saved"
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
