package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

// TokenResult contains refreshed token data.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

// RefreshGitLabToken refreshes a GitLab OAuth token.
func RefreshGitLabToken(ctx context.Context, cfg auth.ProviderConfig, refreshToken string) (TokenResult, error) {
	if refreshToken == "" {
		return TokenResult{}, errors.New("gitlab refresh token missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com/api/v4"
	}
	oauthBase := strings.TrimSuffix(baseURL, "/api/v4")
	endpoint := oauthBase + "/oauth/token"

	values := url.Values{}
	values.Set("client_id", cfg.OAuth.ClientID)
	values.Set("client_secret", cfg.OAuth.ClientSecret)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	_ = cfg

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return TokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResult{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("gitlab token refresh close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResult{}, fmt.Errorf("gitlab token refresh failed: %s", resp.Status)
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return TokenResult{}, err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return TokenResult{}, errors.New("gitlab access token missing")
	}
	out := TokenResult{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	}
	if out.RefreshToken == "" {
		out.RefreshToken = refreshToken
	}
	return out, nil
}

// RefreshBitbucketToken refreshes a Bitbucket OAuth token.
func RefreshBitbucketToken(ctx context.Context, cfg auth.ProviderConfig, refreshToken string) (TokenResult, error) {
	if refreshToken == "" {
		return TokenResult{}, errors.New("bitbucket refresh token missing")
	}
	endpoint := "https://bitbucket.org/site/oauth2/access_token"

	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	_ = cfg

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return TokenResult{}, err
	}
	req.SetBasicAuth(cfg.OAuth.ClientID, cfg.OAuth.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResult{}, fmt.Errorf("bitbucket token refresh failed: %s", resp.Status)
	}
	var token oauthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return TokenResult{}, err
	}
	token.ExpiresAt = expiryFromToken(token)
	if token.AccessToken == "" {
		return TokenResult{}, errors.New("bitbucket access token missing")
	}
	out := TokenResult{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
	}
	if out.RefreshToken == "" {
		out.RefreshToken = refreshToken
	}
	return out, nil
}

// RefreshSlackToken refreshes a Slack OAuth token when token rotation is enabled.
func RefreshSlackToken(ctx context.Context, cfg auth.ProviderConfig, refreshToken string) (TokenResult, error) {
	if refreshToken == "" {
		return TokenResult{}, errors.New("slack refresh token missing")
	}
	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" {
		return TokenResult{}, errors.New("slack oauth client config missing")
	}
	baseURL := strings.TrimRight(cfg.API.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://slack.com/api"
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/oauth.v2.access"

	values := url.Values{}
	values.Set("client_id", cfg.OAuth.ClientID)
	values.Set("client_secret", cfg.OAuth.ClientSecret)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return TokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResult{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("slack token refresh close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResult{}, fmt.Errorf("slack token refresh failed: %s", resp.Status)
	}
	var payload struct {
		OK           bool   `json:"ok"`
		Error        string `json:"error"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return TokenResult{}, err
	}
	if !payload.OK {
		if payload.Error == "" {
			payload.Error = "unknown_error"
		}
		return TokenResult{}, fmt.Errorf("slack token refresh failed: %s", payload.Error)
	}
	if payload.AccessToken == "" {
		return TokenResult{}, errors.New("slack access token missing")
	}
	token := oauthToken{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresIn: payload.ExpiresIn}
	token.ExpiresAt = expiryFromToken(token)
	out := TokenResult{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, ExpiresAt: token.ExpiresAt}
	if out.RefreshToken == "" {
		out.RefreshToken = refreshToken
	}
	return out, nil
}
