package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/relaymesh/githook/pkg/auth"
	ghprovider "github.com/relaymesh/githook/pkg/providers/github"
	"github.com/relaymesh/githook/pkg/storage"
)

func (h *Handler) handleGitHubApp(w http.ResponseWriter, r *http.Request, logger *log.Logger, cfg auth.ProviderConfig) {
	stateValue := decodeState(r.URL.Query().Get("state"))
	installationID := r.URL.Query().Get("installation_id")
	code := r.URL.Query().Get("code")
	if installationID == "" {
		http.Error(w, "missing installation_id", http.StatusBadRequest)
		return
	}
	storeCtx := storage.WithTenant(r.Context(), stateValue.TenantID)
	cfg, instanceKey, instanceRedirect := h.resolveInstanceConfig(storeCtx, "github", stateValue.InstanceKey, cfg)

	var token oauthToken
	var err error
	if code != "" {
		if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
			http.Error(w, "oauth client config missing", http.StatusInternalServerError)
			return
		}
		redirectURL := callbackURL(r, "github", h.Endpoint)
		token, err = exchangeGitHubToken(r.Context(), cfg, code, redirectURL)
		if err != nil {
			logger.Printf("github oauth exchange failed: %v", err)
			http.Error(w, "token exchange failed", http.StatusBadRequest)
			return
		}
	}

	accessToken := token.AccessToken
	refreshToken := token.RefreshToken
	warning := ""

	accountID, accountName, err := resolveGitHubAccount(r.Context(), cfg, installationID)
	if err != nil {
		logger.Printf("github account resolve failed: %v", err)
	}

	record := storage.InstallRecord{
		TenantID:            stateValue.TenantID,
		Provider:            "github",
		AccountID:           accountID,
		AccountName:         accountName,
		InstallationID:      installationID,
		ProviderInstanceKey: instanceKey,
		AccessToken:         accessToken,
		RefreshToken:        refreshToken,
		ExpiresAt:           token.ExpiresAt,
		MetadataJSON:        token.MetadataJSON(),
	}
	if storeAvailable(h.Store) {
		if existing, err := h.Store.GetInstallationByInstallationID(storeCtx, "github", installationID); err == nil && existing != nil {
			record.EnterpriseID = existing.EnterpriseID
			record.EnterpriseSlug = existing.EnterpriseSlug
			record.EnterpriseName = existing.EnterpriseName
		}
	}

	if warning == "" && storeAvailable(h.Store) {
		if record.InstallationID == resolveExistingInstallationID(storeCtx, h.Store, "github", accountID, instanceKey) && code == "" {
			warning = "already installed"
		}
	}

	if storeAvailable(h.Store) {
		logUpsertAttempt(logger, record, accessToken)
		if err := h.Store.UpsertInstallation(storeCtx, record); err != nil {
			logger.Printf("github installation upsert failed: %v", err)
			warning = "install record not saved"
		}
		dedupeInstallations(storeCtx, h.Store, "github", accountID, instanceKey, record.InstallationID)
	}

	if namespaceStoreAvailable(h.NamespaceStore) {
		syncCtx := storage.WithTenant(context.Background(), stateValue.TenantID)
		asyncNamespaceSync(syncCtx, logger, "github", func(ctx context.Context) error {
			return SyncGitHubNamespaces(ctx, h.NamespaceStore, cfg, installationID, accountID, instanceKey)
		})
	}

	params := map[string]string{
		"provider":        "github",
		"account_id":      accountID,
		"account_name":    accountName,
		"installation_id": installationID,
		"warning":         warning,
	}
	h.redirectOrJSON(w, r, params, instanceRedirect)
}

func exchangeGitHubToken(ctx context.Context, cfg auth.ProviderConfig, code, redirectURL string) (oauthToken, error) {
	base := strings.TrimRight(cfg.API.BaseURL, "/")
	oauthBase := "https://github.com"
	if base != "" && base != "https://api.github.com" {
		oauthBase = strings.TrimSuffix(base, "/api/v3")
		oauthBase = strings.TrimSuffix(oauthBase, "/api")
	}
	endpoint := oauthBase + "/login/oauth/access_token"

	values := url.Values{}
	values.Set("client_id", cfg.OAuth.ClientID)
	values.Set("client_secret", cfg.OAuth.ClientSecret)
	values.Set("code", code)
	if redirectURL != "" {
		values.Set("redirect_uri", redirectURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthToken{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("github token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthToken{}, fmt.Errorf("github token exchange failed: %s", resp.Status)
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return oauthToken{}, err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return oauthToken{}, fmt.Errorf("github access token missing response=%+v", token)
	}
	return token, nil
}

func resolveGitHubAccount(ctx context.Context, cfg auth.ProviderConfig, installationID string) (string, string, error) {
	if cfg.App.AppID == 0 ||
		(cfg.App.PrivateKeyPath == "" && cfg.App.PrivateKeyPEM == "") ||
		installationID == "" {
		return "", "", errors.New("github app config missing")
	}
	id, err := strconv.ParseInt(installationID, 10, 64)
	if err != nil {
		return "", "", err
	}
	account, err := ghprovider.FetchInstallationAccount(ctx, ghprovider.AppConfig{
		AppID:          cfg.App.AppID,
		PrivateKeyPath: cfg.App.PrivateKeyPath,
		PrivateKeyPEM:  cfg.App.PrivateKeyPEM,
		BaseURL:        cfg.API.BaseURL,
	}, id)
	if err != nil {
		return "", "", err
	}
	return account.ID, account.Name, nil
}
