package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/auth/oidc"
	"github.com/relaymesh/relaymesh/pkg/core"
)

type tokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

var oauth2Cache tokenCache

func oauth2Token(ctx context.Context) (string, error) {
	cfg, ok, err := loadOAuth2Config()
	if err != nil || !ok {
		return "", err
	}
	return oauth2TokenFromConfig(ctx, cfg)
}

func oauth2TokenFromConfig(ctx context.Context, cfg auth.OAuth2Config) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}
	_, err := normalizeOAuth2Mode(cfg.Mode)
	if err != nil {
		return "", err
	}
	oauth2Cache.mu.Lock()
	if oauth2Cache.token != "" && time.Now().Before(oauth2Cache.expiresAt) {
		token := oauth2Cache.token
		oauth2Cache.mu.Unlock()
		return token, nil
	}
	oauth2Cache.mu.Unlock()

	cachePath := tokenCachePath()
	cacheKey := oidc.CacheKey(cfg)
	if cachePath != "" {
		token, expiresAt, ok, err := oidc.LoadCachedToken(cachePath, cacheKey)
		if err == nil && ok && token != "" && time.Now().Before(expiresAt.Add(-30*time.Second)) {
			return token, nil
		}
	}

	token, err := oidc.ClientCredentialsToken(ctx, cfg)
	if err != nil {
		return "", err
	}
	expiry := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if token.ExpiresIn == 0 {
		expiry = time.Now().Add(30 * time.Minute)
	}
	oauth2Cache.mu.Lock()
	oauth2Cache.token = token.AccessToken
	oauth2Cache.expiresAt = expiry
	oauth2Cache.mu.Unlock()
	if cachePath != "" {
		_ = oidc.StoreCachedToken(cachePath, cacheKey, token.AccessToken, expiry)
	}
	return token.AccessToken, nil
}

func normalizeOAuth2Mode(mode string) (string, error) {
	value := strings.TrimSpace(strings.ToLower(mode))
	if value == "" || value == "auto" {
		return "client_credentials", nil
	}
	if value == "client_credentials" {
		return value, nil
	}
	return "", fmt.Errorf("unsupported oauth2 mode for worker sdk: %s", mode)
}

func loadOAuth2Config() (auth.OAuth2Config, bool, error) {
	configPath := configPathFromEnv()
	if strings.TrimSpace(configPath) == "" {
		return auth.OAuth2Config{}, false, nil
	}
	cfg, err := core.LoadAppConfig(configPath)
	if err != nil {
		return auth.OAuth2Config{}, false, err
	}
	return cfg.Auth.OAuth2, true, nil
}

func tokenCachePath() string {
	if path := strings.TrimSpace(os.Getenv("github.com/relaymesh/relaymesh_TOKEN_CACHE")); path != "" {
		return path
	}
	path, err := oidc.DefaultCachePath()
	if err != nil {
		return ""
	}
	return path
}

func configPathFromEnv() string {
	if path := strings.TrimSpace(os.Getenv("github.com/relaymesh/relaymesh_CONFIG_PATH")); path != "" {
		return path
	}
	return strings.TrimSpace(os.Getenv("github.com/relaymesh/relaymesh_CONFIG"))
}
