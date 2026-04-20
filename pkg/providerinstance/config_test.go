package providerinstance

import (
	"encoding/json"
	"testing"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func TestNormalizeProviderConfigJSON(t *testing.T) {
	raw := `{
  "Webhook": {"Secret": "devsecret"},
  "App": {"AppID": 139247, "AppSlug": "runner", "PrivateKeyPEM": "pem"},
  "OAuth": {"ClientID": "id", "ClientSecret": "secret", "Scopes": "read:user, repo"},
  "API": {"BaseURL": "https://api.github.com", "WebBaseURL": "https://github.com"}
}`
	normalized, ok := NormalizeProviderConfigJSON(raw)
	if !ok {
		t.Fatalf("expected normalized config")
	}
	var cfg auth.ProviderConfig
	if err := json.Unmarshal([]byte(normalized), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	if cfg.Webhook.Secret != "devsecret" {
		t.Fatalf("expected webhook secret, got %q", cfg.Webhook.Secret)
	}
	if cfg.App.AppID != 139247 || cfg.App.AppSlug != "runner" || cfg.App.PrivateKeyPEM != "pem" {
		t.Fatalf("unexpected app config: %+v", cfg.App)
	}
	if cfg.OAuth.ClientID != "id" || cfg.OAuth.ClientSecret != "secret" {
		t.Fatalf("unexpected oauth config: %+v", cfg.OAuth)
	}
	if len(cfg.OAuth.Scopes) != 2 || cfg.OAuth.Scopes[0] != "read:user" || cfg.OAuth.Scopes[1] != "repo" {
		t.Fatalf("unexpected scopes: %v", cfg.OAuth.Scopes)
	}
	if cfg.API.BaseURL != "https://api.github.com" || cfg.API.WebBaseURL != "https://github.com" {
		t.Fatalf("unexpected api config: %+v", cfg.API)
	}
}

func TestSplitScopes(t *testing.T) {
	scopes := splitScopes("read:user, repo\nadmin")
	if len(scopes) != 3 || scopes[0] != "read:user" || scopes[1] != "repo" || scopes[2] != "admin" {
		t.Fatalf("unexpected scopes: %v", scopes)
	}
}

func TestRecordsFromConfig(t *testing.T) {
	cfg := auth.Config{
		GitHub: auth.ProviderConfig{
			App: auth.AppConfig{AppID: 42},
		},
	}
	records, err := RecordsFromConfig(cfg)
	if err != nil {
		t.Fatalf("records from config: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	record := records[0]
	if record.Provider != "github" {
		t.Fatalf("expected github provider, got %q", record.Provider)
	}
	if !record.Enabled {
		t.Fatalf("expected provider record enabled")
	}
	var parsed auth.ProviderConfig
	if err := json.Unmarshal([]byte(record.ConfigJSON), &parsed); err != nil {
		t.Fatalf("unmarshal record config: %v", err)
	}
	if parsed.App.AppID != 42 {
		t.Fatalf("expected app id 42, got %d", parsed.App.AppID)
	}
}

func TestProviderConfigFromRecord(t *testing.T) {
	raw, err := json.Marshal(auth.ProviderConfig{
		API: auth.APIConfig{BaseURL: "https://api.example.com"},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	record := storage.ProviderInstanceRecord{
		ConfigJSON: string(raw),
		Enabled:    true,
	}
	cfg, err := ProviderConfigFromRecord(record)
	if err != nil {
		t.Fatalf("provider config from record: %v", err)
	}
	if cfg.API.BaseURL != "https://api.example.com" {
		t.Fatalf("expected api base url, got %q", cfg.API.BaseURL)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled true")
	}
}

func TestProviderInstanceHelpers(t *testing.T) {
	t.Run("records from empty config", func(t *testing.T) {
		records, err := RecordsFromConfig(auth.Config{})
		if err != nil {
			t.Fatalf("records from empty config: %v", err)
		}
		if len(records) != 0 {
			t.Fatalf("expected no records for empty config")
		}
	})

	t.Run("normalize invalid config json", func(t *testing.T) {
		if normalized, ok := NormalizeProviderConfigJSON("{"); ok || normalized != "" {
			t.Fatalf("expected invalid json normalize to fail")
		}
	})

	t.Run("provider config from invalid record json", func(t *testing.T) {
		_, err := ProviderConfigFromRecord(storage.ProviderInstanceRecord{ConfigJSON: "{"})
		if err == nil {
			t.Fatalf("expected invalid config json error")
		}
	})

	t.Run("provider config parses numeric oauth client id", func(t *testing.T) {
		record := storage.ProviderInstanceRecord{
			ConfigJSON: `{"oauth":{"client_id":1897120182515.1096,"client_secret":"secret","scopes":["chat:write"]}}`,
		}
		cfg, err := ProviderConfigFromRecord(record)
		if err != nil {
			t.Fatalf("provider config from numeric client id: %v", err)
		}
		if cfg.OAuth.ClientID != "1897120182515.1096" {
			t.Fatalf("unexpected oauth client id: %q", cfg.OAuth.ClientID)
		}
		if cfg.OAuth.ClientSecret != "secret" {
			t.Fatalf("unexpected oauth client secret: %q", cfg.OAuth.ClientSecret)
		}
	})

	t.Run("hasProviderConfig coverage", func(t *testing.T) {
		if hasProviderConfig(auth.ProviderConfig{}) {
			t.Fatalf("expected empty provider config to be false")
		}
		if !hasProviderConfig(auth.ProviderConfig{Enabled: true}) {
			t.Fatalf("expected enabled config to be true")
		}
		if !hasProviderConfig(auth.ProviderConfig{Webhook: auth.WebhookConfig{Secret: "x"}}) {
			t.Fatalf("expected webhook config to be true")
		}
		if !hasProviderConfig(auth.ProviderConfig{OAuth: auth.OAuthConfig{ClientID: "id"}}) {
			t.Fatalf("expected oauth config to be true")
		}
	})
}
