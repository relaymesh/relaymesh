package bitbucket

import (
	"os"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestNormalizeBaseURL(t *testing.T) {
	if got := normalizeBaseURL(" https://api.bitbucket.org/2.0/ "); got != "https://api.bitbucket.org/2.0" {
		t.Fatalf("unexpected base url: %q", got)
	}
	if got := normalizeBaseURL(" "); got != "" {
		t.Fatalf("expected empty base url, got %q", got)
	}
}

func TestNewTokenClientRequiresToken(t *testing.T) {
	if _, err := NewTokenClient(auth.ProviderConfig{}, ""); err == nil {
		t.Fatalf("expected token required error")
	}
}

func TestNewTokenClientSetsBaseURL(t *testing.T) {
	t.Setenv("BITBUCKET_API_BASE_URL", "")

	client, err := NewTokenClient(auth.ProviderConfig{
		API: auth.APIConfig{BaseURL: "https://bitbucket.example.com/"},
	}, "token-123")
	if err != nil {
		t.Fatalf("new token client: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client")
	}
	if got := os.Getenv("BITBUCKET_API_BASE_URL"); got != "https://bitbucket.example.com" {
		t.Fatalf("unexpected BITBUCKET_API_BASE_URL=%q", got)
	}
}
