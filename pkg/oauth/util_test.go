package oauth

import (
	"net/http"
	"testing"
)

func TestCallbackURLUsesEndpoint(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/auth/github/callback", nil)
	endpoint := "https://githook.example.com/"
	got := callbackURL(req, "github", endpoint)
	if got != "https://githook.example.com/auth/github/callback" {
		t.Fatalf("unexpected callback url: %q", got)
	}
}

func TestCallbackURLUsesForwardedHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://internal/auth/github/callback", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "edge.example.com")
	got := callbackURL(req, "github", "")
	if got != "https://edge.example.com/auth/github/callback" {
		t.Fatalf("unexpected callback url: %q", got)
	}
}

func TestProviderFromPath(t *testing.T) {
	if provider := providerFromPath("/auth/github/callback"); provider != "github" {
		t.Fatalf("expected github, got %q", provider)
	}
	if provider := providerFromPath("/auth/gitlab/callback/"); provider != "gitlab" {
		t.Fatalf("expected gitlab, got %q", provider)
	}
	if provider := providerFromPath("/auth/bitbucket/callback"); provider != "bitbucket" {
		t.Fatalf("expected bitbucket, got %q", provider)
	}
	if provider := providerFromPath("/auth/slack/callback"); provider != "slack" {
		t.Fatalf("expected slack, got %q", provider)
	}
	if provider := providerFromPath("/auth/atlassian/callback"); provider != "atlassian" {
		t.Fatalf("expected atlassian, got %q", provider)
	}
	if provider := providerFromPath("/auth/jira/callback"); provider != "atlassian" {
		t.Fatalf("expected atlassian alias, got %q", provider)
	}
	if provider := providerFromPath("/auth/unknown/callback"); provider != "" {
		t.Fatalf("expected empty provider, got %q", provider)
	}
}

func TestRandomID(t *testing.T) {
	value := randomID()
	if len(value) != 32 {
		t.Fatalf("expected 32-char hex id, got %q", value)
	}
}
