package webhook

import (
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	providerspkg "github.com/relaymesh/relaymesh/pkg/providers"
)

func TestWebhookProviderMetadata(t *testing.T) {
	gh := gitHubProvider{}
	if gh.Name() != "github" {
		t.Fatalf("unexpected github provider name")
	}
	if path := gh.WebhookPath(auth.ProviderConfig{Webhook: auth.WebhookConfig{Path: "/webhooks/github"}}); path != "/webhooks/github" {
		t.Fatalf("unexpected github webhook path: %q", path)
	}
	if def := gh.Definition(); def.Type != providerspkg.TypeSCM || !def.HasCapability(providerspkg.CapabilityAPIClient) {
		t.Fatalf("unexpected github provider definition: %+v", def)
	}
	if fields := gh.WebhookLogFields(auth.ProviderConfig{App: auth.AppConfig{AppID: 42}}); fields == "" {
		t.Fatalf("expected github log fields")
	}

	gl := gitLabProvider{}
	if gl.Name() != "gitlab" {
		t.Fatalf("unexpected gitlab provider name")
	}
	if def := gl.Definition(); def.Type != providerspkg.TypeSCM {
		t.Fatalf("unexpected gitlab provider definition: %+v", def)
	}
	if fields := gl.WebhookLogFields(auth.ProviderConfig{}); fields != "" {
		t.Fatalf("expected empty gitlab log fields")
	}

	bb := bitbucketProvider{}
	if bb.Name() != "bitbucket" {
		t.Fatalf("unexpected bitbucket provider name")
	}
	if def := bb.Definition(); def.Type != providerspkg.TypeSCM {
		t.Fatalf("unexpected bitbucket provider definition: %+v", def)
	}
	if fields := bb.WebhookLogFields(auth.ProviderConfig{}); fields != "" {
		t.Fatalf("expected empty bitbucket log fields")
	}

	sl := slackProvider{}
	if sl.Name() != "slack" {
		t.Fatalf("unexpected slack provider name")
	}
	if def := sl.Definition(); def.Type != providerspkg.TypeCollaboration || !def.HasCapability(providerspkg.CapabilityWebhookReceive) {
		t.Fatalf("unexpected slack provider definition: %+v", def)
	}
}
