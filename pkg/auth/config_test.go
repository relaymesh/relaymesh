package auth

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOAuthConfigScopesString(t *testing.T) {
	input := "client_id: test\nclient_secret: secret\nscopes: read:user,repo\n"
	var cfg OAuthConfig
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal oauth config: %v", err)
	}
	want := []string{"read:user", "repo"}
	if !reflect.DeepEqual(cfg.Scopes, want) {
		t.Fatalf("expected scopes %v, got %v", want, cfg.Scopes)
	}
}

func TestOAuth2ConfigScopesString(t *testing.T) {
	input := "enabled: true\nrequired_scopes: openid profile\nscopes: read:user\n"
	var cfg OAuth2Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal oauth2 config: %v", err)
	}
	wantRequired := []string{"openid", "profile"}
	if !reflect.DeepEqual(cfg.RequiredScopes, wantRequired) {
		t.Fatalf("expected required scopes %v, got %v", wantRequired, cfg.RequiredScopes)
	}
	wantScopes := []string{"read:user"}
	if !reflect.DeepEqual(cfg.Scopes, wantScopes) {
		t.Fatalf("expected scopes %v, got %v", wantScopes, cfg.Scopes)
	}
}

func TestProviderHelpers(t *testing.T) {
	t.Run("is github provider", func(t *testing.T) {
		if !IsGitHubProvider(" GitHub ") {
			t.Fatalf("expected github provider true")
		}
		if IsGitHubProvider("gitlab") {
			t.Fatalf("expected github provider false")
		}
	})

	t.Run("provider config for built-in and extra", func(t *testing.T) {
		cfg := Config{
			GitHub:    ProviderConfig{Key: "gh"},
			GitLab:    ProviderConfig{Key: "gl"},
			Bitbucket: ProviderConfig{Key: "bb"},
			Slack:     ProviderConfig{Key: "sl"},
			Extra: map[string]ProviderConfig{
				"custom-provider": {Key: "custom"},
			},
		}

		if got, ok := cfg.ProviderConfigFor("github"); !ok || got.Key != "gh" {
			t.Fatalf("unexpected github config: %+v ok=%v", got, ok)
		}
		if got, ok := cfg.ProviderConfigFor("gitlab"); !ok || got.Key != "gl" {
			t.Fatalf("unexpected gitlab config: %+v ok=%v", got, ok)
		}
		if got, ok := cfg.ProviderConfigFor("bitbucket"); !ok || got.Key != "bb" {
			t.Fatalf("unexpected bitbucket config: %+v ok=%v", got, ok)
		}
		if got, ok := cfg.ProviderConfigFor("slack"); !ok || got.Key != "sl" {
			t.Fatalf("unexpected slack config: %+v ok=%v", got, ok)
		}
		if got, ok := cfg.ProviderConfigFor("custom_provider"); !ok || got.Key != "custom" {
			t.Fatalf("unexpected extra config: %+v ok=%v", got, ok)
		}

		cfg.Extra = nil
		if got, ok := cfg.ProviderConfigFor("missing"); ok || got.Key != "" {
			t.Fatalf("expected missing provider not found")
		}
	})
}
