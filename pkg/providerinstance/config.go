package providerinstance

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// RecordsFromConfig converts provider config into instance records.
func RecordsFromConfig(cfg auth.Config) ([]storage.ProviderInstanceRecord, error) {
	out := make([]storage.ProviderInstanceRecord, 0, 3)
	if record, ok, err := instanceRecordFromConfig("github", cfg.GitHub); err != nil {
		return nil, err
	} else if ok {
		out = append(out, record)
	}
	if record, ok, err := instanceRecordFromConfig("gitlab", cfg.GitLab); err != nil {
		return nil, err
	} else if ok {
		out = append(out, record)
	}
	if record, ok, err := instanceRecordFromConfig("bitbucket", cfg.Bitbucket); err != nil {
		return nil, err
	} else if ok {
		out = append(out, record)
	}
	return out, nil
}

// ProviderConfigFromRecord returns a provider config from an instance record.
func ProviderConfigFromRecord(record storage.ProviderInstanceRecord) (auth.ProviderConfig, error) {
	var cfg auth.ProviderConfig
	if err := unmarshalConfig(record.ConfigJSON, &cfg); err != nil {
		return auth.ProviderConfig{}, err
	}
	cfg.Enabled = record.Enabled
	return cfg, nil
}

// NormalizeProviderConfigJSON converts provider config JSON (snake or camel case)
// into the canonical JSON representation used by Go structs.
func NormalizeProviderConfigJSON(raw string) (string, bool) {
	var cfg auth.ProviderConfig
	if err := unmarshalConfig(raw, &cfg); err != nil {
		return "", false
	}
	cfg.Key = ""
	normalized, err := json.Marshal(cfg)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(normalized)), true
}

func instanceRecord(provider string, cfg auth.ProviderConfig) (storage.ProviderInstanceRecord, error) {
	cfg.Key = ""
	raw, err := json.Marshal(cfg)
	if err != nil {
		return storage.ProviderInstanceRecord{}, err
	}
	return storage.ProviderInstanceRecord{
		Provider:   provider,
		Key:        "",
		ConfigJSON: string(raw),
		Enabled:    true,
	}, nil
}

func instanceRecordFromConfig(provider string, cfg auth.ProviderConfig) (storage.ProviderInstanceRecord, bool, error) {
	if !hasProviderConfig(cfg) {
		return storage.ProviderInstanceRecord{}, false, nil
	}
	record, err := instanceRecord(provider, cfg)
	if err != nil {
		return storage.ProviderInstanceRecord{}, false, err
	}
	return record, true, nil
}

func hasProviderConfig(cfg auth.ProviderConfig) bool {
	if cfg.Enabled {
		return true
	}
	if strings.TrimSpace(cfg.Webhook.Path) != "" || strings.TrimSpace(cfg.Webhook.Secret) != "" {
		return true
	}
	if cfg.App.AppID != 0 ||
		strings.TrimSpace(cfg.App.PrivateKeyPath) != "" ||
		strings.TrimSpace(cfg.App.PrivateKeyPEM) != "" ||
		strings.TrimSpace(cfg.App.AppSlug) != "" {
		return true
	}
	if strings.TrimSpace(cfg.OAuth.ClientID) != "" || strings.TrimSpace(cfg.OAuth.ClientSecret) != "" || len(cfg.OAuth.Scopes) > 0 {
		return true
	}
	if strings.TrimSpace(cfg.API.BaseURL) != "" || strings.TrimSpace(cfg.API.WebBaseURL) != "" {
		return true
	}
	return false
}

func unmarshalConfig(raw string, target interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if target == nil {
		return errors.New("target is nil")
	}
	if cfg, ok := target.(*auth.ProviderConfig); ok {
		return unmarshalProviderConfig(raw, cfg)
	}
	return json.Unmarshal([]byte(raw), target)
}

type providerConfigWire struct {
	Enabled    *bool           `json:"enabled"`
	EnabledAlt *bool           `json:"Enabled"`
	Key        *string         `json:"key"`
	KeyAlt     *string         `json:"Key"`
	Webhook    json.RawMessage `json:"webhook"`
	WebhookAlt json.RawMessage `json:"Webhook"`
	App        json.RawMessage `json:"app"`
	AppAlt     json.RawMessage `json:"App"`
	OAuth      json.RawMessage `json:"oauth"`
	OAuthAlt   json.RawMessage `json:"OAuth"`
	API        json.RawMessage `json:"api"`
	APIAlt     json.RawMessage `json:"API"`
}

type webhookConfigWire struct {
	Path      *string `json:"path"`
	PathAlt   *string `json:"Path"`
	Secret    *string `json:"secret"`
	SecretAlt *string `json:"Secret"`
}

type appConfigWire struct {
	AppID             *int64  `json:"app_id"`
	AppIDAlt          *int64  `json:"AppID"`
	PrivateKeyPath    *string `json:"private_key_path"`
	PrivateKeyPEM     *string `json:"private_key_pem"`
	PrivateKeyPathAlt *string `json:"PrivateKeyPath"`
	PrivateKeyPEMAlt  *string `json:"PrivateKeyPEM"`
	AppSlug           *string `json:"app_slug"`
	AppSlugAlt        *string `json:"AppSlug"`
}

type oauthConfigWire struct {
	ClientID        *string         `json:"client_id"`
	ClientIDAlt     *string         `json:"ClientID"`
	ClientSecret    *string         `json:"client_secret"`
	ClientSecretAlt *string         `json:"ClientSecret"`
	Scopes          json.RawMessage `json:"scopes"`
	ScopesAlt       json.RawMessage `json:"Scopes"`
}

type apiConfigWire struct {
	BaseURL       *string `json:"base_url"`
	BaseURLAlt    *string `json:"BaseURL"`
	WebBaseURL    *string `json:"web_base_url"`
	WebBaseURLAlt *string `json:"WebBaseURL"`
}

func unmarshalProviderConfig(raw string, target *auth.ProviderConfig) error {
	if target == nil {
		return errors.New("target is nil")
	}
	var wire providerConfigWire
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return err
	}
	if wire.Enabled != nil {
		target.Enabled = *wire.Enabled
	} else if wire.EnabledAlt != nil {
		target.Enabled = *wire.EnabledAlt
	}
	if wire.Key != nil {
		target.Key = *wire.Key
	} else if wire.KeyAlt != nil {
		target.Key = *wire.KeyAlt
	}

	webhookRaw := wire.Webhook
	if len(webhookRaw) == 0 {
		webhookRaw = wire.WebhookAlt
	}
	if len(webhookRaw) > 0 {
		var wh webhookConfigWire
		if err := json.Unmarshal(webhookRaw, &wh); err != nil {
			return err
		}
		if wh.Path != nil {
			target.Webhook.Path = *wh.Path
		} else if wh.PathAlt != nil {
			target.Webhook.Path = *wh.PathAlt
		}
		if wh.Secret != nil {
			target.Webhook.Secret = *wh.Secret
		} else if wh.SecretAlt != nil {
			target.Webhook.Secret = *wh.SecretAlt
		}
	}

	appRaw := wire.App
	if len(appRaw) == 0 {
		appRaw = wire.AppAlt
	}
	if len(appRaw) > 0 {
		var app appConfigWire
		if err := json.Unmarshal(appRaw, &app); err != nil {
			return err
		}
		if app.AppID != nil {
			target.App.AppID = *app.AppID
		} else if app.AppIDAlt != nil {
			target.App.AppID = *app.AppIDAlt
		}
		if app.PrivateKeyPath != nil {
			target.App.PrivateKeyPath = *app.PrivateKeyPath
		} else if app.PrivateKeyPathAlt != nil {
			target.App.PrivateKeyPath = *app.PrivateKeyPathAlt
		}
		if app.PrivateKeyPEM != nil {
			target.App.PrivateKeyPEM = *app.PrivateKeyPEM
		} else if app.PrivateKeyPEMAlt != nil {
			target.App.PrivateKeyPEM = *app.PrivateKeyPEMAlt
		}
		if app.AppSlug != nil {
			target.App.AppSlug = *app.AppSlug
		} else if app.AppSlugAlt != nil {
			target.App.AppSlug = *app.AppSlugAlt
		}
	}

	oauthRaw := wire.OAuth
	if len(oauthRaw) == 0 {
		oauthRaw = wire.OAuthAlt
	}
	if len(oauthRaw) > 0 {
		var oauth oauthConfigWire
		if err := json.Unmarshal(oauthRaw, &oauth); err != nil {
			return err
		}
		if oauth.ClientID != nil {
			target.OAuth.ClientID = *oauth.ClientID
		} else if oauth.ClientIDAlt != nil {
			target.OAuth.ClientID = *oauth.ClientIDAlt
		}
		if oauth.ClientSecret != nil {
			target.OAuth.ClientSecret = *oauth.ClientSecret
		} else if oauth.ClientSecretAlt != nil {
			target.OAuth.ClientSecret = *oauth.ClientSecretAlt
		}
		scopesRaw := oauth.Scopes
		if len(scopesRaw) == 0 {
			scopesRaw = oauth.ScopesAlt
		}
		if len(scopesRaw) > 0 {
			var scopes []string
			if err := json.Unmarshal(scopesRaw, &scopes); err == nil {
				target.OAuth.Scopes = scopes
			} else {
				var rawScopes string
				if err := json.Unmarshal(scopesRaw, &rawScopes); err == nil {
					target.OAuth.Scopes = splitScopes(rawScopes)
				}
			}
		}
	}

	apiRaw := wire.API
	if len(apiRaw) == 0 {
		apiRaw = wire.APIAlt
	}
	if len(apiRaw) > 0 {
		var api apiConfigWire
		if err := json.Unmarshal(apiRaw, &api); err != nil {
			return err
		}
		if api.BaseURL != nil {
			target.API.BaseURL = *api.BaseURL
		} else if api.BaseURLAlt != nil {
			target.API.BaseURL = *api.BaseURLAlt
		}
		if api.WebBaseURL != nil {
			target.API.WebBaseURL = *api.WebBaseURL
		} else if api.WebBaseURLAlt != nil {
			target.API.WebBaseURL = *api.WebBaseURLAlt
		}
	}

	return nil
}

func splitScopes(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
