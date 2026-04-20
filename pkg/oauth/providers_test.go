package oauth

import (
	"net/http"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestOAuthProviderMetadata(t *testing.T) {
	gh := gitHubProvider{}
	if gh.Name() != "github" || gh.CallbackPath() != "/auth/github/callback" {
		t.Fatalf("unexpected github provider metadata")
	}
	if url, err := gh.AuthorizeURL(nil, auth.ProviderConfig{App: auth.AppConfig{AppSlug: "app"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected github authorize url")
	}

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/start", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "example.com")

	gl := gitLabProvider{}
	if gl.Name() != "gitlab" || gl.CallbackPath() != "/auth/gitlab/callback" {
		t.Fatalf("unexpected gitlab provider metadata")
	}
	if url, err := gl.AuthorizeURL(req, auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected gitlab authorize url")
	}

	bb := bitbucketProvider{}
	if bb.Name() != "bitbucket" || bb.CallbackPath() != "/auth/bitbucket/callback" {
		t.Fatalf("unexpected bitbucket provider metadata")
	}
	if url, err := bb.AuthorizeURL(req, auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected bitbucket authorize url")
	}

	sl := slackProvider{}
	if sl.Name() != "slack" || sl.CallbackPath() != "/auth/slack/callback" {
		t.Fatalf("unexpected slack provider metadata")
	}
	if url, err := sl.AuthorizeURL(req, auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected slack authorize url")
	}

	jr := jiraProvider{}
	if jr.Name() != "jira" || jr.CallbackPath() != "/auth/jira/callback" {
		t.Fatalf("unexpected jira provider metadata")
	}
	if url, err := jr.AuthorizeURL(req, auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected jira authorize url")
	}

	at := atlassianProvider{}
	if at.Name() != "atlassian" || at.CallbackPath() != "/auth/atlassian/callback" {
		t.Fatalf("unexpected atlassian provider metadata")
	}
	if url, err := at.AuthorizeURL(req, auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}, "state", ""); err != nil || url == "" {
		t.Fatalf("expected atlassian authorize url")
	}
}
