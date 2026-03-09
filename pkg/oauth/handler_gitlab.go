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
	"strconv"
	"strings"

	"github.com/relaymesh/githook/pkg/auth"
	"github.com/relaymesh/githook/pkg/storage"
)

func (h *Handler) handleGitLab(w http.ResponseWriter, r *http.Request, logger *log.Logger, cfg auth.ProviderConfig) {
	stateValue := decodeState(r.URL.Query().Get("state"))
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	storeCtx := storage.WithTenant(r.Context(), stateValue.TenantID)
	cfg, instanceKey, instanceRedirect := h.resolveInstanceConfig(storeCtx, "gitlab", stateValue.InstanceKey, cfg)

	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		http.Error(w, "oauth client config missing", http.StatusInternalServerError)
		return
	}

	redirectURL := callbackURL(r, "gitlab", h.Endpoint)
	token, err := exchangeGitLabToken(r.Context(), cfg, code, redirectURL)
	if err != nil {
		logger.Printf("gitlab oauth exchange failed: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	accountID, accountName, err := resolveGitLabAccount(r.Context(), cfg, token.AccessToken)
	if err != nil {
		logger.Printf("gitlab account resolve failed: %v", err)
	}

	warning := ""
	if warning == "" && storeAvailable(h.Store) {
		if resolveExistingInstallationID(storeCtx, h.Store, "gitlab", accountID, instanceKey) != "" {
			warning = "already installed"
		}
	}

	record := storage.InstallRecord{
		TenantID:            stateValue.TenantID,
		Provider:            "gitlab",
		AccountID:           accountID,
		AccountName:         accountName,
		AccessToken:         token.AccessToken,
		RefreshToken:        token.RefreshToken,
		ExpiresAt:           token.ExpiresAt,
		ProviderInstanceKey: instanceKey,
		MetadataJSON:        token.MetadataJSON(),
	}

	if storeAvailable(h.Store) {
		logUpsertAttempt(logger, record, token.AccessToken)
		if err := h.Store.UpsertInstallation(storeCtx, record); err != nil {
			logger.Printf("gitlab installation upsert failed: %v", err)
			warning = "install record not saved"
		}
		dedupeInstallations(storeCtx, h.Store, "gitlab", accountID, instanceKey, record.InstallationID)
	}

	if namespaceStoreAvailable(h.NamespaceStore) {
		syncCtx := storage.WithTenant(context.Background(), stateValue.TenantID)
		asyncNamespaceSync(syncCtx, logger, "gitlab", func(ctx context.Context) error {
			return SyncGitLabNamespaces(ctx, h.NamespaceStore, cfg, token.AccessToken, accountID, record.InstallationID, instanceKey)
		})
	}

	params := map[string]string{
		"provider":     "gitlab",
		"account_id":   accountID,
		"account_name": accountName,
		"warning":      warning,
	}
	h.redirectOrJSON(w, r, params, instanceRedirect)
}

func exchangeGitLabToken(ctx context.Context, cfg auth.ProviderConfig, code, redirectURL string) (oauthToken, error) {
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	oauthBase := strings.TrimSuffix(baseURL, "/api/v4")
	endpoint := oauthBase + "/oauth/token"

	values := url.Values{}
	values.Set("client_id", cfg.OAuth.ClientID)
	values.Set("client_secret", cfg.OAuth.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	if redirectURL != "" {
		values.Set("redirect_uri", redirectURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthToken{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("gitlab token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthToken{}, fmt.Errorf("gitlab token exchange failed: %s", resp.Status)
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oauthToken{}, err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return oauthToken{}, errors.New("gitlab access token missing")
	}
	return token, nil
}

func resolveGitLabAccount(ctx context.Context, cfg auth.ProviderConfig, accessToken string) (string, string, error) {
	if accessToken == "" {
		return "", "", errors.New("gitlab access token missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	endpoint := baseURL + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("gitlab user lookup close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("gitlab user lookup failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	name := payload.Username
	if name == "" {
		name = payload.Name
	}
	return strconv.FormatInt(payload.ID, 10), name, nil
}
