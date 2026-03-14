package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/auth/oidc"
	"github.com/relaymesh/relaymesh/pkg/core"
)

func connectClientOptions() ([]connect.ClientOption, error) {
	interceptor := validate.NewInterceptor()
	opts := []connect.ClientOption{
		connect.WithInterceptors(interceptor),
	}
	cfg, err := loadCLIConfig()
	if err != nil {
		return opts, err
	}
	apiBaseURL = resolveEndpoint(cfg)
	if apiKey := strings.TrimSpace(os.Getenv("RELAYMESH_API_KEY")); apiKey != "" {
		opts = append(opts, connect.WithInterceptors(apiKeyHeaderInterceptor(apiKey)))
		return opts, nil
	}
	if !oauth2EnabledForCLI(cfg.Auth.OAuth2) {
		return opts, nil
	}
	token, err := cliToken(context.Background(), cfg)
	if err != nil {
		return opts, err
	}
	if token != "" {
		opts = append(opts, connect.WithInterceptors(authHeaderInterceptor(token)))
	}
	return opts, nil
}

func authHeaderInterceptor(token string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if token != "" {
				req.Header().Set("Authorization", "Bearer "+token)
			}
			return next(ctx, req)
		}
	}
}

func apiKeyHeaderInterceptor(apiKey string) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if apiKey != "" {
				req.Header().Set("x-api-key", apiKey)
			}
			return next(ctx, req)
		}
	}
}

func loadCLIConfig() (core.AppConfig, error) {
	if strings.TrimSpace(configPath) == "" {
		return core.AppConfig{}, errors.New("config path not set")
	}
	return core.LoadAppConfig(configPath)
}

func resolveEndpoint(cfg core.AppConfig) string {
	if strings.TrimSpace(apiBaseURL) != "" {
		return strings.TrimSpace(apiBaseURL)
	}
	if strings.TrimSpace(cfg.Endpoint) != "" {
		return strings.TrimSpace(cfg.Endpoint)
	}
	return "http://localhost:8080"
}

func cliToken(ctx context.Context, cfg core.AppConfig) (string, error) {
	if token := strings.TrimSpace(os.Getenv("RELAYMESH_AUTH_TOKEN")); token != "" {
		return token, nil
	}
	cachePath := tokenCachePath()
	cacheKey := oidc.CacheKey(cfg.Auth.OAuth2)
	if cachePath != "" {
		token, expiresAt, ok, err := oidc.LoadCachedToken(cachePath, cacheKey)
		if err == nil && ok && token != "" && time.Now().Before(expiresAt.Add(-30*time.Second)) {
			return token, nil
		}
	}
	if strings.ToLower(strings.TrimSpace(cfg.Auth.OAuth2.Mode)) == "auth_code" && cfg.Auth.OAuth2.ClientSecret == "" {
		return "", errors.New("auth_code mode requires login; set RELAYMESH_AUTH_TOKEN or store a token with relaymesh auth store")
	}
	resp, err := oidc.ClientCredentialsToken(ctx, cfg.Auth.OAuth2)
	if err != nil {
		return "", err
	}
	if cachePath != "" {
		expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
		if resp.ExpiresIn == 0 {
			expiresAt = time.Now().Add(30 * time.Minute)
		}
		_ = oidc.StoreCachedToken(cachePath, cacheKey, resp.AccessToken, expiresAt)
	}
	return resp.AccessToken, nil
}

func tokenCachePath() string {
	if path := strings.TrimSpace(os.Getenv("RELAYMESH_TOKEN_CACHE")); path != "" {
		return path
	}
	path, err := oidc.DefaultCachePath()
	if err != nil {
		return ""
	}
	return path
}

func oauth2EnabledForCLI(cfg auth.OAuth2Config) bool {
	if cfg.Enabled {
		return true
	}
	if strings.TrimSpace(cfg.Issuer) != "" {
		return true
	}
	if strings.TrimSpace(cfg.Audience) != "" {
		return true
	}
	if strings.TrimSpace(cfg.ClientID) != "" || strings.TrimSpace(cfg.ClientSecret) != "" {
		return true
	}
	if strings.TrimSpace(cfg.AuthorizeURL) != "" || strings.TrimSpace(cfg.TokenURL) != "" || strings.TrimSpace(cfg.JWKSURL) != "" {
		return true
	}
	if len(cfg.RequiredScopes) > 0 || len(cfg.Scopes) > 0 {
		return true
	}
	return false
}
