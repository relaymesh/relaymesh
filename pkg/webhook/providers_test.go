package webhook

import (
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

func TestWebhookProviderMetadata(t *testing.T) {
	gh := gitHubProvider{}
	if gh.Name() != "github" {
		t.Fatalf("unexpected github provider name")
	}
	if path := gh.WebhookPath(auth.ProviderConfig{Webhook: auth.WebhookConfig{Path: "/webhooks/github"}}); path != "/webhooks/github" {
		t.Fatalf("unexpected github webhook path: %q", path)
	}
	if fields := gh.WebhookLogFields(auth.ProviderConfig{App: auth.AppConfig{AppID: 42}}); fields == "" {
		t.Fatalf("expected github log fields")
	}

	gl := gitLabProvider{}
	if gl.Name() != "gitlab" {
		t.Fatalf("unexpected gitlab provider name")
	}
	if fields := gl.WebhookLogFields(auth.ProviderConfig{}); fields != "" {
		t.Fatalf("expected empty gitlab log fields")
	}

	bb := bitbucketProvider{}
	if bb.Name() != "bitbucket" {
		t.Fatalf("unexpected bitbucket provider name")
	}
	if fields := bb.WebhookLogFields(auth.ProviderConfig{}); fields != "" {
		t.Fatalf("expected empty bitbucket log fields")
	}
}
