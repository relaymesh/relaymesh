package oidc

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
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

type Token struct {
	AccessToken string
	ExpiresIn   int64
}

func ResolveEndpoints(ctx context.Context, cfg auth.OAuth2Config) (authorizeURL, tokenURL, jwksURL string, err error) {
	authorizeURL = strings.TrimSpace(cfg.AuthorizeURL)
	tokenURL = strings.TrimSpace(cfg.TokenURL)
	jwksURL = strings.TrimSpace(cfg.JWKSURL)
	if authorizeURL != "" && tokenURL != "" && jwksURL != "" {
		return authorizeURL, tokenURL, jwksURL, nil
	}
	if cfg.Issuer == "" {
		return authorizeURL, tokenURL, jwksURL, errors.New("issuer is required for discovery")
	}
	discovery, err := Discover(ctx, cfg.Issuer)
	if err != nil {
		return authorizeURL, tokenURL, jwksURL, err
	}
	if authorizeURL == "" {
		authorizeURL = discovery.AuthorizationEndpoint
	}
	if tokenURL == "" {
		tokenURL = discovery.TokenEndpoint
	}
	if jwksURL == "" {
		jwksURL = discovery.JWKSURI
	}
	return authorizeURL, tokenURL, jwksURL, nil
}

func ClientCredentialsToken(ctx context.Context, cfg auth.OAuth2Config) (Token, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return Token{}, errors.New("client_id and client_secret are required")
	}
	_, tokenURL, _, err := ResolveEndpoints(ctx, cfg)
	if err != nil {
		return Token{}, err
	}
	if tokenURL == "" {
		return Token{}, errors.New("token_url is required")
	}
	values := url.Values{}
	values.Set("grant_type", "client_credentials")
	values.Set("client_id", cfg.ClientID)
	values.Set("client_secret", cfg.ClientSecret)
	if strings.TrimSpace(cfg.Audience) != "" {
		values.Set("audience", strings.TrimSpace(cfg.Audience))
	}
	if len(cfg.RequiredScopes) > 0 {
		values.Set("scope", strings.Join(cfg.RequiredScopes, " "))
	} else if len(cfg.Scopes) > 0 {
		values.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("oidc token exchange close failed: %v", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readErrorBody(resp.Body)
		if body != "" {
			return Token{}, fmt.Errorf("token exchange failed: %s (%s)", resp.Status, body)
		}
		return Token{}, fmt.Errorf("token exchange failed: %s", resp.Status)
	}
	type response struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	var payload response
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Token{}, err
	}
	if payload.AccessToken == "" {
		return Token{}, errors.New("access_token missing")
	}
	return Token(payload), nil
}

func readErrorBody(r io.Reader) string {
	const maxBody = 4096
	raw, err := io.ReadAll(io.LimitReader(r, maxBody))
	if err != nil {
		return ""
	}
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return ""
	}
	if sanitized := sanitizeJSON(body); sanitized != "" {
		body = sanitized
	}
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\r", " ")
	return body
}

func sanitizeJSON(raw string) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	delete(payload, "access_token")
	delete(payload, "refresh_token")
	delete(payload, "id_token")
	if len(payload) == 0 {
		return ""
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}
