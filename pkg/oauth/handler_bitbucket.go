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

	"github.com/relaymesh/githook/pkg/auth"
	"github.com/relaymesh/githook/pkg/storage"
)

func (h *Handler) handleBitbucket(w http.ResponseWriter, r *http.Request, logger *log.Logger, cfg auth.ProviderConfig) {
	stateValue := decodeState(r.URL.Query().Get("state"))
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	storeCtx := storage.WithTenant(r.Context(), stateValue.TenantID)
	cfg, instanceKey, instanceRedirect := h.resolveInstanceConfig(storeCtx, "bitbucket", stateValue.InstanceKey, cfg)

	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		http.Error(w, "oauth client config missing", http.StatusInternalServerError)
		return
	}

	redirectURL := callbackURL(r, "bitbucket", h.Endpoint)
	token, err := exchangeBitbucketToken(r.Context(), cfg, code, redirectURL)
	if err != nil {
		logger.Printf("bitbucket oauth exchange failed: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	accountID, accountName, err := resolveBitbucketAccount(r.Context(), cfg, token.AccessToken)
	if err != nil {
		logger.Printf("bitbucket account resolve failed: %v", err)
	}

	warning := ""
	if warning == "" && storeAvailable(h.Store) {
		if resolveExistingInstallationID(storeCtx, h.Store, "bitbucket", accountID, instanceKey) != "" {
			warning = "already installed"
		}
	}

	record := storage.InstallRecord{
		TenantID:            stateValue.TenantID,
		Provider:            "bitbucket",
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
			logger.Printf("bitbucket installation upsert failed: %v", err)
			warning = "install record not saved"
		}
		dedupeInstallations(storeCtx, h.Store, "bitbucket", accountID, instanceKey, record.InstallationID)
	}

	if namespaceStoreAvailable(h.NamespaceStore) {
		syncCtx := storage.WithTenant(context.Background(), stateValue.TenantID)
		asyncNamespaceSync(syncCtx, logger, "bitbucket", func(ctx context.Context) error {
			return SyncBitbucketNamespaces(ctx, h.NamespaceStore, cfg, token.AccessToken, accountID, record.InstallationID, instanceKey)
		})
	}

	params := map[string]string{
		"provider":     "bitbucket",
		"account_id":   accountID,
		"account_name": accountName,
		"warning":      warning,
	}
	h.redirectOrJSON(w, r, params, instanceRedirect)
}

func exchangeBitbucketToken(ctx context.Context, cfg auth.ProviderConfig, code, redirectURL string) (oauthToken, error) {
	endpoint := "https://bitbucket.org/site/oauth2/access_token"

	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	if redirectURL != "" {
		values.Set("redirect_uri", redirectURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthToken{}, err
	}
	req.SetBasicAuth(cfg.OAuth.ClientID, cfg.OAuth.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthToken{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("bitbucket token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return oauthToken{}, fmt.Errorf("bitbucket token exchange failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oauthToken{}, err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return oauthToken{}, errors.New("bitbucket access token missing")
	}
	return token, nil
}

func resolveBitbucketAccount(ctx context.Context, cfg auth.ProviderConfig, accessToken string) (string, string, error) {
	if accessToken == "" {
		return "", "", errors.New("bitbucket access token missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.bitbucket.org/2.0"
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
			log.Printf("bitbucket user lookup close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("bitbucket user lookup failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		UUID        string `json:"uuid"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	name := payload.DisplayName
	if name == "" {
		name = payload.Nickname
	}
	if name == "" {
		name = payload.Username
	}
	return payload.UUID, name, nil
}
