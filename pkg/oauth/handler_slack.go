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

func (h *Handler) handleSlack(w http.ResponseWriter, r *http.Request, logger *log.Logger, cfg auth.ProviderConfig) {
	stateValue := decodeState(r.URL.Query().Get("state"))
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	storeCtx := storage.WithTenant(r.Context(), stateValue.TenantID)
	cfg, instanceKey, instanceRedirect := h.resolveInstanceConfig(storeCtx, auth.ProviderSlack, stateValue.InstanceKey, cfg)

	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		http.Error(w, "oauth client config missing", http.StatusInternalServerError)
		return
	}

	redirectURL := callbackURL(r, auth.ProviderSlack, h.Endpoint)
	token, accountID, accountName, metadataJSON, err := exchangeSlackToken(r.Context(), cfg, code, redirectURL)
	if err != nil {
		logger.Printf("slack oauth exchange failed: %v", err)
		http.Error(w, "token exchange failed", http.StatusBadRequest)
		return
	}

	installationID := resolveExistingInstallationID(storeCtx, h.Store, auth.ProviderSlack, accountID, instanceKey)
	if installationID == "" {
		installationID = accountID
	}

	warning := ""
	record := storage.InstallRecord{
		TenantID:            stateValue.TenantID,
		Provider:            auth.ProviderSlack,
		AccountID:           accountID,
		AccountName:         accountName,
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
			logger.Printf("slack installation upsert failed: %v", err)
			warning = "install record not saved"
		}
		dedupeInstallations(storeCtx, h.Store, auth.ProviderSlack, accountID, instanceKey, record.InstallationID)
	}

	params := map[string]string{
		"provider":        auth.ProviderSlack,
		"account_id":      accountID,
		"account_name":    accountName,
		"installation_id": installationID,
		"warning":         warning,
	}
	h.redirectOrJSON(w, r, params, instanceRedirect)
}

func exchangeSlackToken(ctx context.Context, cfg auth.ProviderConfig, code, redirectURL string) (oauthToken, string, string, string, error) {
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://slack.com/api"
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/oauth.v2.access"

	values := url.Values{}
	values.Set("client_id", cfg.OAuth.ClientID)
	values.Set("client_secret", cfg.OAuth.ClientSecret)
	values.Set("code", code)
	if redirectURL != "" {
		values.Set("redirect_uri", redirectURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return oauthToken{}, "", "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthToken{}, "", "", "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("slack token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return oauthToken{}, "", "", "", fmt.Errorf("slack token exchange failed: %s body=%s", resp.Status, strings.TrimSpace(string(body)))
	}

	var payload slackOAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return oauthToken{}, "", "", "", err
	}
	if !payload.OK {
		if payload.Error == "" {
			payload.Error = "unknown_error"
		}
		return oauthToken{}, "", "", "", fmt.Errorf("slack oauth error: %s", payload.Error)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return oauthToken{}, "", "", "", errors.New("slack access token missing")
	}

	teamID := strings.TrimSpace(payload.Team.ID)
	if teamID == "" {
		teamID = strings.TrimSpace(payload.Enterprise.ID)
	}
	if teamID == "" {
		return oauthToken{}, "", "", "", errors.New("slack team id missing")
	}
	teamName := strings.TrimSpace(payload.Team.Name)
	if teamName == "" {
		teamName = strings.TrimSpace(payload.Enterprise.Name)
	}

	token := oauthToken{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		ExpiresIn:    payload.ExpiresIn,
		TokenType:    payload.TokenType,
		Scope:        payload.Scope,
	}
	token.ExpiresAt = expiryFromToken(token)

	metadata := map[string]interface{}{
		"token_type":             payload.TokenType,
		"scope":                  payload.Scope,
		"bot_user_id":            payload.BotUserID,
		"app_id":                 payload.AppID,
		"incoming_webhook_url":   payload.IncomingWebhook.URL,
		"incoming_webhook_ch":    payload.IncomingWebhook.Channel,
		"incoming_webhook_ch_id": payload.IncomingWebhook.ChannelID,
		"authed_user_id":         payload.AuthedUser.ID,
		"authed_user_scope":      payload.AuthedUser.Scope,
		"enterprise_id":          payload.Enterprise.ID,
		"enterprise_name":        payload.Enterprise.Name,
	}
	rawMeta, _ := json.Marshal(metadata)

	return token, teamID, teamName, string(rawMeta), nil
}

type slackOAuthResponse struct {
	OK           bool   `json:"ok"`
	Error        string `json:"error"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	BotUserID    string `json:"bot_user_id"`
	AppID        string `json:"app_id"`
	Team         struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"team"`
	Enterprise struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"enterprise"`
	AuthedUser struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	} `json:"authed_user"`
	IncomingWebhook struct {
		URL       string `json:"url"`
		Channel   string `json:"channel"`
		ChannelID string `json:"channel_id"`
	} `json:"incoming_webhook"`
}
