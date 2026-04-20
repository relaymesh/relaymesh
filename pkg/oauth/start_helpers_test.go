package oauth

import (
	"context"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestStartURLHelpers(t *testing.T) {
	if got := githubWebBase(auth.ProviderConfig{}); got != "https://github.com" {
		t.Fatalf("unexpected github web base: %q", got)
	}
	if got := githubWebBase(auth.ProviderConfig{API: auth.APIConfig{BaseURL: "https://github.example.com/api/v3"}}); got != "https://github.example.com" {
		t.Fatalf("unexpected github enterprise web base: %q", got)
	}
	if got := gitlabWebBase(auth.ProviderConfig{}); got != "https://gitlab.com" {
		t.Fatalf("unexpected gitlab web base: %q", got)
	}
	if got := gitlabWebBase(auth.ProviderConfig{API: auth.APIConfig{BaseURL: "https://gitlab.example.com/api/v4"}}); got != "https://gitlab.example.com" {
		t.Fatalf("unexpected gitlab enterprise web base: %q", got)
	}

	if url, err := githubInstallURL(auth.ProviderConfig{}, "state"); err == nil || url != "" {
		t.Fatalf("expected github install url error")
	}
	if url, err := githubInstallURL(auth.ProviderConfig{App: auth.AppConfig{AppSlug: "my-app"}}, "state"); err != nil || url == "" {
		t.Fatalf("expected github install url")
	}

	if url, err := gitlabAuthorizeURL(auth.ProviderConfig{}, "state", "https://callback"); err == nil || url != "" {
		t.Fatalf("expected gitlab authorize error")
	}
	if url, err := gitlabAuthorizeURL(auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id", Scopes: []string{"read"}}}, "state", "https://callback"); err != nil || url == "" {
		t.Fatalf("expected gitlab authorize url")
	}

	if url, err := bitbucketAuthorizeURL(auth.ProviderConfig{}, "state", "https://callback"); err == nil || url != "" {
		t.Fatalf("expected bitbucket authorize error")
	}
	if url, err := bitbucketAuthorizeURL(auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", "https://callback"); err != nil || url == "" {
		t.Fatalf("expected bitbucket authorize url")
	}

	if url, err := slackAuthorizeURL(auth.ProviderConfig{}, "state", "https://callback"); err == nil || url != "" {
		t.Fatalf("expected slack authorize error")
	}
	if url, err := slackAuthorizeURL(auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id", Scopes: []string{"chat:write", "channels:read"}}}, "state", "https://callback"); err != nil || url == "" {
		t.Fatalf("expected slack authorize url")
	}

	if url, err := jiraAuthorizeURL(auth.ProviderConfig{}, "state", "https://callback"); err == nil || url != "" {
		t.Fatalf("expected jira authorize error")
	}
	if url, err := jiraAuthorizeURL(auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id", Scopes: []string{"read:jira-user", "read:jira-work"}}}, "state", "https://callback"); err != nil || url == "" {
		t.Fatalf("expected jira authorize url")
	}

	if got, err := addQueryParam("https://example.com/x", "state", ""); err != nil || got != "https://example.com/x" {
		t.Fatalf("unexpected addQueryParam empty value result: %q err=%v", got, err)
	}
	if got, err := addQueryParam("https://example.com/x", "state", "abc"); err != nil || got == "" {
		t.Fatalf("expected addQueryParam result, got=%q err=%v", got, err)
	}

	if len(randomState()) != 32 {
		t.Fatalf("expected randomState to be 32 hex chars")
	}
}

func TestStartResolveProviderConfigFallback(t *testing.T) {
	h := &StartHandler{Providers: auth.Config{GitHub: auth.ProviderConfig{Key: "gh"}}}
	ctx := storage.WithTenant(context.Background(), "tenant-a")
	cfg, key := h.resolveProviderConfig(ctx, auth.ProviderGitHub, "")
	if cfg.Key != "gh" || key != "" {
		t.Fatalf("unexpected fallback provider config: %+v key=%q", cfg, key)
	}

	if cfg, key = h.resolveProviderConfig(ctx, auth.ProviderGitHub, "instance-1"); key != "instance-1" || cfg.Key != "gh" {
		t.Fatalf("unexpected fallback provider+instance config: %+v key=%q", cfg, key)
	}
}
