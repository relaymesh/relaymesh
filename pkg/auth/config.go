package auth

// Config contains provider configuration for webhooks and SCM auth.
type Config struct {
	GitHub    ProviderConfig            `yaml:"github"`
	GitLab    ProviderConfig            `yaml:"gitlab"`
	Bitbucket ProviderConfig            `yaml:"bitbucket"`
	Slack     ProviderConfig            `yaml:"slack"`
	Extra     map[string]ProviderConfig `yaml:"extra"`
}

// ProviderConfigFor returns a provider config by name, including extras.
func (c Config) ProviderConfigFor(provider string) (ProviderConfig, bool) {
	provider = NormalizeProviderName(provider)
	switch provider {
	case ProviderGitHub:
		return c.GitHub, true
	case ProviderGitLab:
		return c.GitLab, true
	case ProviderBitbucket:
		return c.Bitbucket, true
	case ProviderSlack:
		return c.Slack, true
	default:
		if c.Extra == nil {
			return ProviderConfig{}, false
		}
		if cfg, ok := c.Extra[provider]; ok {
			return cfg, true
		}
		for key, cfg := range c.Extra {
			if NormalizeProviderName(key) == provider {
				return cfg, true
			}
		}
		return ProviderConfig{}, false
	}
}

// ProviderConfig contains webhook and auth configuration for a provider.
type ProviderConfig struct {
	Enabled bool   `yaml:"enabled"` // Deprecated: webhooks are always enabled.
	Key     string `yaml:"key"`

	Webhook WebhookConfig `yaml:"webhook"`
	App     AppConfig     `yaml:"app"`
	OAuth   OAuthConfig   `yaml:"oauth"`
	API     APIConfig     `yaml:"api"`
}

// WebhookConfig contains inbound webhook settings.
type WebhookConfig struct {
	Path   string `yaml:"path"`
	Secret string `yaml:"secret"`
}

// AppConfig contains GitHub App settings.
type AppConfig struct {
	AppID          int64  `yaml:"app_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
	PrivateKeyPEM  string `yaml:"private_key_pem" json:"PrivateKeyPEM,omitempty"`
	AppSlug        string `yaml:"app_slug"`
}

// OAuthConfig contains OAuth settings (future OAuth2 expansion).
type OAuthConfig struct {
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"scopes"`
}

// APIConfig contains provider API and web base URLs.
type APIConfig struct {
	BaseURL    string `yaml:"base_url"`
	WebBaseURL string `yaml:"web_base_url"`
}
